package spark

import (
	"context"
	"testing"

	apipb "github.com/michelangelo-ai/michelangelo/proto/api"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/spark"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

type SparkModuleTestSuite struct {
	suite.Suite
	service.TestSuite
	env *service.TestEnvironment
}

func TestSparkModuleSuite(t *testing.T) {
	suite.Run(t, new(SparkModuleTestSuite))
}

func (s *SparkModuleTestSuite) SetupTest() {
	s.env = s.NewTestEnvironment(s.T(), &service.TestEnvironmentParams{
		RootDirectory: "testdata",
		Plugins: map[string]service.IPlugin{
			"spark": Plugin,
		},
	})
}

func (s *SparkModuleTestSuite) TearDownTest() {
	s.env.Cadence.AssertExpectations(s.T())
	s.env.Temporal.AssertExpectations(s.T())
}

func (s *SparkModuleTestSuite) TestCreateJobSuccessfully() {
	env := s.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(spark.Activities.CreateSparkJob)
	env.RegisterActivity(spark.Activities.SensorSparkJob)

	sparkJob := &v2pb.SparkJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-spark-job",
			Namespace: "default",
		},
		Spec: v2pb.SparkJobSpec{
			MainClass: "test",
		},
	}

	var createJobReq v2pb.CreateSparkJobRequest
	env.OnActivity(spark.Activities.CreateSparkJob, mock.Anything, mock.Anything).Once().
		Run(func(args mock.Arguments) {
			createJobReq = args.Get(1).(v2pb.CreateSparkJobRequest)
		}).
		Return(func(ctx context.Context, req v2pb.CreateSparkJobRequest) (*v2pb.CreateSparkJobResponse, error) {
			return &v2pb.CreateSparkJobResponse{
				SparkJob: sparkJob,
			}, nil
		})

	s.env.Cadence.ExecuteFunction("/test.star", "test_create_job", nil, nil, nil)
	require := s.Require()
	var res any
	err := s.env.Cadence.GetResult(&res)
	require.NoError(err)
	require.NotNil(createJobReq.SparkJob.Spec.User)
	require.NotNil(createJobReq.SparkJob.Spec.Driver)
	require.NotNil(createJobReq.SparkJob.Spec.Driver.Pod.Resource)
	require.NotNil(createJobReq.SparkJob.Spec.Executor)
	require.NotNil(createJobReq.SparkJob.Spec.Executor.Pod.Resource)
}

func (s *SparkModuleTestSuite) TestSensorJobSuccessfully() {
	env := s.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(spark.Activities.SensorSparkJob)

	//var sensorJobReq v2pb.GetSparkJobRequest
	env.OnActivity(spark.Activities.SensorSparkJob, mock.Anything, mock.Anything).
		Return(func(ctx context.Context, req v2pb.GetSparkJobRequest) (*spark.SensorSparkJobResponse, error) {
			// Simulate a Spark job's status condition
			return &spark.SensorSparkJobResponse{
				SparkJob: &v2pb.SparkJob{
					Status: v2pb.SparkJobStatus{
						StatusConditions: []*apipb.Condition{
							{
								Type:   "Succeeded",
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
					},
				},
				Terminal: true,
			}, nil
		})

	s.env.Cadence.ExecuteFunction("/test.star", "test_sensor_job", nil, nil, nil)
	require := s.Require()
	var res any
	require.NoError(s.env.Cadence.GetResult(&res))
}
