package trigger

import (
	"testing"

	api "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGenerateBatchRerunPipelineRunRequest(t *testing.T) {
	target := &api.ResourceIdentifier{Namespace: "test-namespace", Name: "failed-run-1"}
	triggerRun := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-namespace",
			Name:      "test-trigger",
		},
		Spec: v2pb.TriggerRunSpec{
			Pipeline: &api.ResourceIdentifier{
				Namespace: "test-namespace",
				Name:      "test-pipeline",
			},
			Revision: &api.ResourceIdentifier{
				Namespace: "test-namespace",
				Name:      "rev-1",
			},
			Actor: &v2pb.UserInfo{Name: "test-user"},
			Trigger: &v2pb.Trigger{
				TriggerType: &v2pb.Trigger_BatchRerun{
					BatchRerun: &v2pb.BatchRerun{
						PipelineRuns: []*api.ResourceIdentifier{target},
						ResumeFrom:   []string{"feature_gen"},
						ResumeUpTo:   []string{"train_model"},
					},
				},
			},
		},
	}

	req := generateBatchRerunPipelineRunRequest(triggerRun, "new-run-name", target)

	pr := req.PipelineRun
	assert.Equal(t, "new-run-name", pr.Name)
	assert.Equal(t, "test-namespace", pr.Namespace)
	assert.Equal(t, "test-pipeline", pr.Spec.Pipeline.Name)
	assert.Equal(t, "rev-1", pr.Spec.Revision.Name)
	assert.Equal(t, "test-user", pr.Spec.Actor.Name)
	assert.Equal(t, target, pr.Spec.Resume.PipelineRun)
	assert.Equal(t, []string{"feature_gen"}, pr.Spec.Resume.ResumeFrom)
	assert.Equal(t, []string{"train_model"}, pr.Spec.Resume.ResumeUpTo)
	assert.Nil(t, pr.Spec.Input)
	assert.Equal(t, "test-trigger", pr.Labels[TriggerredByLabel])
	assert.Equal(t, "test-pipeline", pr.Labels[PipelineNameLabel])
}
