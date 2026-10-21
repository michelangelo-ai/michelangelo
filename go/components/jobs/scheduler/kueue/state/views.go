package state

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// cohortFields maps each kueue.x-k8s.io API version the state reads to the
// ClusterQueue field naming its cohort: v1beta2 renamed spec.cohort.
var cohortFields = map[string][]string{
	"v1beta1": {"spec", "cohort"},
	"v1beta2": {"spec", "cohortName"},
}

// parseClusterQueue reads the view of a ClusterQueue of the given API version.
func parseClusterQueue(obj *unstructured.Unstructured, apiVersion string) (*ClusterQueueView, error) {
	cohortField, ok := cohortFields[apiVersion]
	if !ok {
		return nil, fmt.Errorf("unsupported Kueue API version %q", apiVersion)
	}
	cq, err := clusterQueueView(obj.Object, cohortField)
	if err != nil {
		return nil, fmt.Errorf("ClusterQueue %s: %w", obj.GetName(), err)
	}
	cq.Name = obj.GetName()
	return cq, nil
}

func clusterQueueView(obj map[string]interface{}, cohortField []string) (*ClusterQueueView, error) {
	cq := &ClusterQueueView{Nominal: corev1.ResourceList{}, Reserved: corev1.ResourceList{}}
	var err error
	if cq.Cohort, err = stringField(obj, cohortField...); err != nil {
		return nil, err
	}
	if cq.StopPolicy, err = stringField(obj, "spec", "stopPolicy"); err != nil {
		return nil, err
	}

	groups, err := objectsField(obj, "spec", "resourceGroups")
	if err != nil {
		return nil, err
	}
	covered := map[string]bool{}
	for _, group := range groups {
		names, err := stringsField(group, "coveredResources")
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			if !covered[name] {
				covered[name] = true
				cq.CoveredResources = append(cq.CoveredResources, corev1.ResourceName(name))
			}
		}
		flavors, err := objectsField(group, "flavors")
		if err != nil {
			return nil, err
		}
		cq.FlavorCount += len(flavors)
		for _, flavor := range flavors {
			if err := addQuantities(cq.Nominal, flavor, "nominalQuota"); err != nil {
				return nil, err
			}
		}
	}

	reservations, err := objectsField(obj, "status", "flavorsReservation")
	if err != nil {
		return nil, err
	}
	for _, reservation := range reservations {
		if err := addQuantities(cq.Reserved, reservation, "total"); err != nil {
			return nil, err
		}
	}

	pending, err := int32Field(obj, "status", "pendingWorkloads")
	if err != nil {
		return nil, err
	}
	cq.PendingWorkloads = pending
	return cq, nil
}

// parseLocalQueue reads the ClusterQueue a LocalQueue feeds and whether its
// Active condition is True.
func parseLocalQueue(obj *unstructured.Unstructured) (clusterQueue string, active bool, err error) {
	if clusterQueue, err = stringField(obj.Object, "spec", "clusterQueue"); err != nil {
		return "", false, fmt.Errorf("LocalQueue %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	conditions, err := objectsField(obj.Object, "status", "conditions")
	if err != nil {
		return "", false, fmt.Errorf("LocalQueue %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	for _, condition := range conditions {
		if condition["type"] == "Active" {
			active = condition["status"] == "True"
		}
	}
	return clusterQueue, active, nil
}

// addQuantities adds the quantity in the field of each of the resources listed
// under obj's resources to total.
func addQuantities(total corev1.ResourceList, obj map[string]interface{}, field string) error {
	resources, err := objectsField(obj, "resources")
	if err != nil {
		return err
	}
	for _, r := range resources {
		name, err := stringField(r, "name")
		if err != nil {
			return err
		}
		q, err := quantityField(r, field)
		if err != nil {
			return fmt.Errorf("resource %s: %w", name, err)
		}
		sum := total[corev1.ResourceName(name)]
		sum.Add(q)
		total[corev1.ResourceName(name)] = sum
	}
	return nil
}

// field returns the value at path in obj; nil if it is absent.
func field(obj map[string]interface{}, path ...string) (interface{}, error) {
	v, _, err := unstructured.NestedFieldNoCopy(obj, path...)
	return v, err
}

func stringField(obj map[string]interface{}, path ...string) (string, error) {
	v, err := field(obj, path...)
	if err != nil || v == nil {
		return "", err
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s is %T, not a string", strings.Join(path, "."), v)
	}
	return s, nil
}

func stringsField(obj map[string]interface{}, path ...string) ([]string, error) {
	v, err := field(obj, path...)
	if err != nil || v == nil {
		return nil, err
	}
	items, ok := v.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%s is %T, not a list", strings.Join(path, "."), v)
	}
	strs := make([]string, len(items))
	for i, item := range items {
		if strs[i], ok = item.(string); !ok {
			return nil, fmt.Errorf("%s[%d] is %T, not a string", strings.Join(path, "."), i, item)
		}
	}
	return strs, nil
}

func objectsField(obj map[string]interface{}, path ...string) ([]map[string]interface{}, error) {
	v, err := field(obj, path...)
	if err != nil || v == nil {
		return nil, err
	}
	items, ok := v.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%s is %T, not a list", strings.Join(path, "."), v)
	}
	objs := make([]map[string]interface{}, len(items))
	for i, item := range items {
		if objs[i], ok = item.(map[string]interface{}); !ok {
			return nil, fmt.Errorf("%s[%d] is %T, not an object", strings.Join(path, "."), i, item)
		}
	}
	return objs, nil
}

// quantityField reads a quantity, which Kueue's CRDs take as an integer or a
// string; zero if it is absent.
func quantityField(obj map[string]interface{}, path ...string) (resource.Quantity, error) {
	v, err := field(obj, path...)
	if err != nil || v == nil {
		return resource.Quantity{}, err
	}
	switch q := v.(type) {
	case string:
		return resource.ParseQuantity(q)
	case int64:
		return *resource.NewQuantity(q, resource.DecimalSI), nil
	case float64:
		return resource.ParseQuantity(strconv.FormatFloat(q, 'f', -1, 64))
	default:
		return resource.Quantity{}, fmt.Errorf("%s is %T, not a quantity", strings.Join(path, "."), v)
	}
}

func int32Field(obj map[string]interface{}, path ...string) (int32, error) {
	v, err := field(obj, path...)
	if err != nil || v == nil {
		return 0, err
	}
	var n int64
	switch i := v.(type) {
	case int64:
		n = i
	case float64:
		if i != math.Trunc(i) {
			return 0, fmt.Errorf("%s is %v, not an integer", strings.Join(path, "."), i)
		}
		n = int64(i)
	default:
		return 0, fmt.Errorf("%s is %T, not an integer", strings.Join(path, "."), v)
	}
	if n < math.MinInt32 || n > math.MaxInt32 {
		return 0, fmt.Errorf("%s is %d, out of range", strings.Join(path, "."), n)
	}
	return int32(n), nil
}
