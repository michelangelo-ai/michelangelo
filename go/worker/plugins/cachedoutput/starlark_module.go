package cachedoutput

import (
	"fmt"
	"time"

	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/star"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/gogo/protobuf/types"

	"github.com/michelangelo-ai/michelangelo/go/worker/activities/cachedoutput"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.starlark.net/starlark"
)

var _ starlark.HasAttrs = (*module)(nil)

type module struct {
	attributes map[string]starlark.Value
}

func newModule() starlark.Value {
	m := &module{}
	m.attributes = map[string]starlark.Value{
		"get":   starlark.NewBuiltin("get", m.get).BindReceiver(m),
		"put":   starlark.NewBuiltin("put", m.put).BindReceiver(m),
		"query": starlark.NewBuiltin("query", m.query).BindReceiver(m),
	}
	return m
}

func (r *module) String() string                        { return pluginID }
func (r *module) Type() string                          { return pluginID }
func (r *module) Freeze()                               {}
func (r *module) Truth() starlark.Bool                  { return true }
func (r *module) Hash() (uint32, error)                 { return 0, fmt.Errorf("no-hash") }
func (r *module) Attr(n string) (starlark.Value, error) { return r.attributes[n], nil }
func (r *module) AttrNames() []string                   { return ext.SortedKeys(r.attributes) }

var properties = map[string]star.PropertyFactory{}

// These are some error reasons
const (
	_errorReasonUnpackArgs           = "UnpackArgsError"
	_errorReasonConvertStarlarkValue = "ConvertStarlarkValueError"
)

// get returns a cachedoutput obj by namespace and name
//
//	get(namespace=namespace, name=name) -> cachedoutput
//
//	  namespace: the namespace of the cachedoutput
//	  name: the name of the cachedoutput
//
//	  return: dict: cachedoutput crd as a dict
func (r *module) get(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := service.GetContext(t)
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

	request := v2pb.GetCachedOutputRequest{
		Namespace: namespace,
		Name:      name,
	}
	response := v2pb.GetCachedOutputResponse{}
	retryPolicy := utils.CadenceDefaultRetryPolicy
	getCtx := workflow.WithRetryPolicy(ctx, retryPolicy)
	if err := workflow.ExecuteActivity(getCtx, cachedoutput.Activities.GetCachedOutput, request).Get(ctx, &response); err != nil {
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
func (r *module) put(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := service.GetContext(t)
	logger := workflow.GetLogger(ctx)
	var cachedOutputDict starlark.Value

	if err := starlark.UnpackArgs("put", args, kwargs,
		"cachedoutput", &cachedOutputDict,
	); err != nil {
		logger.Error(_errorReasonUnpackArgs, ext.ZapError(err)...)
		return nil, err
	}

	co := v2pb.CachedOutput{}
	if err := utils.AsGo(cachedOutputDict, &co); err != nil {
		logger.Error(_errorReasonConvertStarlarkValue, ext.ZapError(err)...)
		return nil, err
	}

	request := v2pb.CreateCachedOutputRequest{
		CachedOutput: &co,
	}
	response := v2pb.CreateCachedOutputResponse{}
	retryPolicy := utils.CadenceDefaultRetryPolicy
	createCtx := workflow.WithRetryPolicy(ctx, retryPolicy)
	if err := workflow.ExecuteActivity(createCtx, cachedoutput.Activities.CreateCachedOutput, request).Get(ctx, &response); err != nil {
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
func (r *module) query(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := service.GetContext(t)
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

	request := v2pb.ListCachedOutputRequest{
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

	response := v2pb.ListCachedOutputResponse{}
	retryPolicy := utils.CadenceDefaultRetryPolicy
	listCtx := workflow.WithRetryPolicy(ctx, retryPolicy)
	if err := workflow.ExecuteActivity(listCtx, cachedoutput.Activities.ListCachedOutput, request).Get(ctx, &response); err != nil {
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
