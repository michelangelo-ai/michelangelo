package cachedoutput

import (
	"fmt"
	"time"

	"code.uber.internal/uberai/michelangelo/starlark/activity/uapi"
	"code.uber.internal/uberai/michelangelo/starlark/plugin/utils"
	"github.com/cadence-workflow/starlark-worker/cadstar"
	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/star"
	"github.com/gogo/protobuf/types"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	"go.starlark.net/starlark"
	"go.uber.org/cadence/workflow"
	v2beta1 "michelangelo/api/v2beta1"
)

// Module is the Ray module.
type Module struct {
	info *workflow.Info
}

var _ starlark.HasAttrs = &Module{}

// String returns the module name.
func (f *Module) String() string { return pluginID }

// Type returns the module type.
func (f *Module) Type() string { return pluginID }

// Freeze freezes the module.
func (f *Module) Freeze() {}

// Truth returns starlark boolean type
func (f *Module) Truth() starlark.Bool { return true }

// Hash returns the module hash.
func (f *Module) Hash() (uint32, error) { return 0, fmt.Errorf("no-hash") }

// Attr returns the module attribute.
func (f *Module) Attr(n string) (starlark.Value, error) { return star.Attr(f, n, builtins, properties) }

// AttrNames returns the module attribute names.
func (f *Module) AttrNames() []string { return star.AttrNames(builtins, properties) }

var properties = map[string]star.PropertyFactory{}

// These are some error reasons
const (
	_errorReasonUnpackArgs           = "UnpackArgsError"
	_errorReasonConvertStarlarkValue = "ConvertStarlarkValueError"
)

var builtins = map[string]*starlark.Builtin{
	"get":   starlark.NewBuiltin("get", get),
	"put":   starlark.NewBuiltin("put", put),
	"query": starlark.NewBuiltin("query", query),
}

// get returns a cachedoutput obj by namespace and name
//
//	get(namespace=namespace, name=name) -> cachedoutput
//
//	  namespace: the namespace of the cachedoutput
//	  name: the name of the cachedoutput
//
//	  return: dict: cachedoutput crd as a dict
func get(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := cadstar.GetContext(t)
	logger := workflow.GetLogger(ctx)
	var namespace string
	var name string

	if err := starlark.UnpackArgs("get", args, kwargs,
		"namespace", &namespace,
		"name", &name,
	); err != nil {
		logger.Error(_errorReasonUnpackArgs, ext.ZapError(err)...)
		return nil, err
	}

	request := v2beta1.GetCachedOutputRequest{
		Namespace: namespace,
		Name:      name,
	}
	response := v2beta1.GetCachedOutputResponse{}
	retryPolicy := utils.CadenceDefaultRetryPolicy
	getCtx := workflow.WithRetryPolicy(ctx, retryPolicy)
	if err := workflow.ExecuteActivity(getCtx, uapi.Activities.GetCachedOutput, request).Get(ctx, &response); err != nil {
		logger.Error("Failed to get CachedOutput", ext.ZapError(err)...)
		return nil, err
	}

	var cachedOutputValue starlark.Value
	if err := utils.AsStar(response.CachedOutput, &cachedOutputValue); err != nil {
		logger.Error(_errorReasonConvertStarlarkValue, ext.ZapError(err)...)
		return nil, err
	}
	return cachedOutputValue, nil
}

// put creates a cachedoutput obj
// put(cachedoutput=cachedoutput) -> cachedoutput
//
//	cachedoutput: a cachedoutput CRD in json format
//	return dict of the created cachedoutput
func put(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := cadstar.GetContext(t)
	logger := workflow.GetLogger(ctx)
	var cachedOutputDict starlark.Value

	if err := starlark.UnpackArgs("put", args, kwargs,
		"cachedoutput", &cachedOutputDict,
	); err != nil {
		logger.Error(_errorReasonUnpackArgs, ext.ZapError(err)...)
		return nil, err
	}

	cachedOutput := v2beta1.CachedOutput{}
	if err := utils.AsGo(cachedOutputDict, &cachedOutput); err != nil {
		logger.Error(_errorReasonConvertStarlarkValue, ext.ZapError(err)...)
		return nil, err
	}

	request := v2beta1.CreateCachedOutputRequest{
		CachedOutput: &cachedOutput,
	}
	response := v2beta1.CreateCachedOutputResponse{}
	retryPolicy := utils.CadenceDefaultRetryPolicy
	createCtx := workflow.WithRetryPolicy(ctx, retryPolicy)
	if err := workflow.ExecuteActivity(createCtx, uapi.Activities.CreateCachedOutput, request).Get(ctx, &response); err != nil {
		logger.Error("Failed to put CachedOutput", ext.ZapError(err)...)
		return nil, err
	}

	var cachedOutputValue starlark.Value
	if err := utils.AsStar(response.CachedOutput, &cachedOutputValue); err != nil {
		logger.Error(_errorReasonConvertStarlarkValue, ext.ZapError(err)...)
		return nil, err
	}

	return cachedOutputValue, nil
}

// query lists cachedoutputs
//
//		match_criterion: a dict indicating the index label/field to match
//		order_by: a list of dict indicating the order by setting
//		lookback_days: int, the number of days to look back
//		limit: int, the number of cachedoutputs to return
//	 return: a list of cachedoutput crd in json dict
func query(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := cadstar.GetContext(t)
	logger := workflow.GetLogger(ctx)
	var matchCritrionDict starlark.Value
	var orderByValue starlark.Value
	var lookbackDaysValue starlark.Int
	var limitValue starlark.Int
	var namespaceValue starlark.String

	if err := starlark.UnpackArgs("query", args, kwargs,
		"namespace", &namespaceValue,
		"match_criterion", &matchCritrionDict,
		"order_by", &orderByValue,
		"lookback_days", &lookbackDaysValue,
		"limit", &limitValue,
	); err != nil {
		logger.Error(_errorReasonUnpackArgs, ext.ZapError(err)...)
		return nil, err
	}

	var namespace string
	if err := utils.AsGo(namespaceValue, &namespace); err != nil {
		logger.Error(_errorReasonConvertStarlarkValue, ext.ZapError(err)...)
		return nil, err
	}

	matchCriterion := map[string]interface{}{}
	if err := utils.AsGo(matchCritrionDict, &matchCriterion); err != nil {
		logger.Error(_errorReasonConvertStarlarkValue, ext.ZapError(err)...)
		return nil, err
	}

	orderBy := []*apipb.OrderBy{}

	if err := utils.AsGo(orderByValue, &orderBy); err != nil {
		logger.Error(_errorReasonConvertStarlarkValue, ext.ZapError(err)...)
		return nil, err
	}

	var lookbackDays int
	var limit int

	if err := utils.AsGo(lookbackDaysValue, &lookbackDays); err != nil {
		logger.Error(_errorReasonConvertStarlarkValue, ext.ZapError(err)...)
		return nil, err
	}

	if err := utils.AsGo(limitValue, &limit); err != nil {
		logger.Error(_errorReasonConvertStarlarkValue, ext.ZapError(err)...)
		return nil, err
	}

	criterion := []*apipb.Criterion{}
	for k, v := range matchCriterion {
		criterion = append(criterion, &apipb.Criterion{
			FieldName: k,
			Operator:  apipb.CRITERION_OPERATOR_EQUAL,
			MatchValue: &types.Any{
				Value: []byte(fmt.Sprintf("%v", v)),
			},
		})
	}

	earliestCreationTime := workflow.Now(ctx).Add(time.Duration(-1) * time.Duration(lookbackDays) * 24 * time.Hour)
	earlistCreationTimestr := earliestCreationTime.Format("2006-01-02")

	createTimeCriterion := &apipb.Criterion{
		FieldName: "cached_output.metadata.creation_timestamp",
		Operator:  apipb.CRITERION_OPERATOR_GREATER_THAN,
		MatchValue: &types.Any{
			Value: []byte(fmt.Sprintf("%s", earlistCreationTimestr)),
		},
	}

	criterion = append(criterion, createTimeCriterion)

	request := v2beta1.ListCachedOutputRequest{
		Namespace: namespace,
		ListOptionsExt: &apipb.ListOptionsExt{
			OrderBy: orderBy,
			Operation: &apipb.CriterionOperation{
				Criterion: criterion,
			},
			Pagination: &apipb.PaginationSpec{
				Limit: int32(limit),
			},
		},
	}

	response := v2beta1.ListCachedOutputResponse{}
	retryPolicy := utils.CadenceDefaultRetryPolicy
	listCtx := workflow.WithRetryPolicy(ctx, retryPolicy)
	if err := workflow.ExecuteActivity(listCtx, uapi.Activities.ListCachedOutput, request).Get(ctx, &response); err != nil {
		logger.Error("Failed to list CachedOutput", ext.ZapError(err)...)
		return nil, err
	}

	var responseValue starlark.Value
	if err := utils.AsStar(response, &responseValue); err != nil {
		logger.Error(_errorReasonConvertStarlarkValue, ext.ZapError(err)...)
		return nil, err
	}
	return responseValue, nil
}
