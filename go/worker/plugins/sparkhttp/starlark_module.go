package sparkhttp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/sparkhttp"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
	"github.com/michelangelo-ai/michelangelo/go/worker/spark"
	"go.starlark.net/starlark"
	"go.uber.org/zap"
)

// Module struct implements starlark.HasAttrs interface
var _ starlark.HasAttrs = (*module)(nil)

var poll int64 = 10

// extractUsernameFromJWT extracts the preferred_username from a JWT token
func extractUsernameFromJWT(token string) (string, error) {
	// Split the JWT token into parts
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid JWT token format")
	}

	// Decode the payload (second part)
	payload := parts[1]
	// Add padding if necessary
	if len(payload)%4 != 0 {
		payload += strings.Repeat("=", 4-len(payload)%4)
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	// Parse the JSON payload
	var claims map[string]interface{}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return "", fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	// Extract preferred_username
	username, ok := claims["preferred_username"].(string)
	if !ok {
		return "", fmt.Errorf("preferred_username not found in JWT token")
	}

	return username, nil
}

type module struct {
	attributes map[string]starlark.Value
}

func newModule() starlark.Value {
	m := &module{}
	m.attributes = map[string]starlark.Value{
		"create_job": starlark.NewBuiltin("create_job", m.createSparkOne).BindReceiver(m),
	}
	return m
}

func (r *module) String() string        { return pluginID }
func (r *module) Type() string          { return pluginID }
func (r *module) Freeze()               {}
func (r *module) Truth() starlark.Bool  { return true }
func (r *module) Hash() (uint32, error) { return starlark.String(pluginID).Hash() }
func (r *module) Attr(name string) (starlark.Value, error) {
	if val, ok := r.attributes[name]; ok {
		return val, nil
	}
	return nil, nil
}
func (r *module) AttrNames() []string {
	return ext.SortedKeys(r.attributes)
}

// createSparkOne creates a new SparkOne via the HTTP API and waits for it to be ready.
func (r *module) createSparkOne(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := service.GetContext(thread)
	logger := workflow.GetLogger(ctx)

	var sparkOneSpec *starlark.Dict
	var userToken string
	if err := starlark.UnpackArgs("create_job", args, kwargs, "spark_one_spec", &sparkOneSpec, "user_token", &userToken); err != nil {
		logger.Error("error unpacking args", zap.Error(err))
		return nil, err
	}

	// Convert the provided sparkOne spec from starlark to worker/spark object
	var sparkOne spark.SparkOne
	if err := utils.AsGo(sparkOneSpec, &sparkOne); err != nil {
		logger.Error("error converting sparkOne spec to worker/spark object", zap.Error(err))
		return nil, err
	}

	// Marshal the sparkOne into the request format expected by activities
	sparkOneBytes, err := json.Marshal(sparkOne)
	if err != nil {
		logger.Error("error marshaling sparkOne", zap.Error(err))
		return nil, err
	}

	var request struct {
		SparkOne  json.RawMessage `json:"sparkOne"`
		UserToken string          `json:"userToken"`
	}
	request.SparkOne = sparkOneBytes
	request.UserToken = userToken

	// Extract username from JWT token
	username, err := extractUsernameFromJWT(userToken)
	if err != nil {
		logger.Error("error extracting username from JWT token", zap.Error(err))
		return nil, err
	}

	// First create SparkOne dependencies
	depsRequest := sparkhttp.CreateSparkOneDepsRequest{
		Username: username,
		Pipeline: sparkOne.Spec.Pipeline,
		JobName:  sparkOne.Metadata.Name,
	}

	var depsResponse sparkhttp.CreateSparkOneDepsResponse
	err = workflow.ExecuteActivity(ctx, sparkhttp.Activities.CreateSparkOneDeps, depsRequest).Get(ctx, &depsResponse)
	if err != nil {
		logger.Error("error executing create deps activity", zap.Error(err))
		return nil, err
	}

	// If pollUrl is not empty, sensor for deps completion
	sensorDepsRequest := sparkhttp.SensorSparkOneDepsRequest{
		PollURL: depsResponse.PollURL,
	}

	var sensorDepsResponse sparkhttp.SensorSparkOneDepsResponse

	// Poll until dependencies are ready
	srp := utils.CadenceDefaultSensorRetryPolicy
	srp.InitialInterval = time.Second * time.Duration(poll)
	sensorDepsCtx := workflow.WithRetryPolicy(ctx, srp)

	err = workflow.ExecuteActivity(sensorDepsCtx, sparkhttp.Activities.SensorSparkOneDeps, sensorDepsRequest).Get(sensorDepsCtx, &sensorDepsResponse)
	if err != nil {
		logger.Error("error executing sensor deps activity", zap.Error(err))
		return nil, err
	}

	if sensorDepsResponse.Status != "success" {
		logger.Error("dependencies failed to build", zap.String("status", sensorDepsResponse.Status), zap.String("msg", sensorDepsResponse.Msg))
		return nil, fmt.Errorf("dependency build failed: %s", sensorDepsResponse.Msg)
	}

	// Execute the create activity
	var createResponse spark.CreateSparkOneResponse
	srp = utils.CadenceDefaultRetryPolicy
	srp.InitialInterval = time.Second * time.Duration(poll)
	createCtx := workflow.WithRetryPolicy(ctx, srp)
	err = workflow.ExecuteActivity(createCtx, sparkhttp.Activities.CreateSparkOne, request).Get(ctx, &createResponse)
	if err != nil {
		logger.Error("error executing create activity", zap.Error(err))
		return nil, err
	}

	name := createResponse.Object["metadata"].(map[string]interface{})["name"].(string)

	// Now poll for the job to be ready
	sensorRequest := struct {
		Name      string `json:"name"`
		UserToken string `json:"userToken"`
	}{
		Name:      name,
		UserToken: userToken,
	}

	// Set up polling with retry policy
	srp = utils.CadenceDefaultSensorRetryPolicy
	srp.InitialInterval = time.Second * time.Duration(poll)
	sensorCtx := workflow.WithRetryPolicy(ctx, srp)

	// Monitor job until it's in a terminal state
	var getResponse spark.GetSparkOneResponse

	if err := workflow.ExecuteActivity(sensorCtx, sparkhttp.Activities.SensorSparkOne, sensorRequest).Get(sensorCtx, &getResponse); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	var result starlark.Value
	if err := utils.AsStar(getResponse.Object, &result); err != nil {
		logger.Error("error converting to Starlark", zap.Error(err))
		return nil, err
	}

	return result, nil
}
