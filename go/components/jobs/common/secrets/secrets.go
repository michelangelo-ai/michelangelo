package secrets

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/shared/gateways/hadooptokenservice"
	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime"
	v2beta1pb "michelangelo/api/v2beta1"
)

// HadoopAlias
const (
	// indicates the DCA NEON cluster.
	_neonServiceAlias = "NEON"
	// indicates the PHX PLATINUM cluster.
	_platinumServiceAlias = "PLATINUM"
	// indicates the Hadoop ROUTER.
	_routerServiceAlias = "ROUTER"
	// cloud lake service alias
	_cloudLakeServiceAlias = "CLOUDLAKE"
)

const (
	_dca = "DCA"
	_phx = "PHX"
)

const (
	// Hadoop HDFS.
	_hdfsService = "HDFS"
	// Hadoop Hive.
	_hiveService = "HIVE"
	// Hadoop Hive Metastore.
	_hiveMetastoreService = "HMS"
	// KeyManagementService.
	_kmsService = "KMS"
	// CloudLake service
	_cloudLakeService = "CLOUD_STORAGE"
	// Path translation service
	_ptsService = "PTS"
)

var _hadoopServiceTokenKindMap = map[string]string{
	_hdfsService:          "HDFS_DELEGATION_TOKEN",
	_hiveService:          "HIVE_DELEGATION_TOKEN",
	_hiveMetastoreService: "HIVE_DELEGATION_TOKEN",
	_kmsService:           "KMS_DELEGATION_TOKEN",
}

var _cloudFSTokenKindMap = map[string]string{
	_cloudLakeService: "CLOUD_STORAGE_DELEGATION_TOKEN",
	_ptsService:       "PTS_DELEGATION_TOKEN",
}

// Token formats required by spark and ray
const (
	_sparkSecretKey = "hadoop.token"
)

const (
	_defaultSecureNsPHX = "ns-platinum-prod-phx"
	_defaultSecureNsDCA = "ns-neon-prod-dca1"
)

// Provider provides functionality for creating training job secrets
type Provider struct {
	tokenGateway hadooptokenservice.Gateway
}

// Params has params for constructor
type Params struct {
	fx.In

	TokenGateway hadooptokenservice.Gateway
}

// Result has the result of the constructor
type Result struct {
	fx.Out

	Provider
}

// New provides new Secrets generator
func New(p Params) Result {
	return Result{
		Provider: Provider{
			tokenGateway: p.TokenGateway,
		},
	}
}

// GetKubeSecretName gets the k8s secret name using the job name
func GetKubeSecretName(jobName string) string {
	return constants.SecretHadoopNamePrefix + jobName
}

func getAliasFromRegion(region string) string {
	var alias string
	switch strings.ToUpper(region) {
	case _dca:
		alias = _neonServiceAlias
	case _phx:
		alias = _platinumServiceAlias
	}
	return alias
}

// GenerateHadoopSecret generates the hadoop delegation token for the current user
func (p Provider) GenerateHadoopSecret(
	ctx context.Context,
	jobObject runtime.Object,
	cluster *v2beta1pb.Cluster) (map[string][]byte, error) {

	resp, err := p.generateHadoopSecret(ctx, jobObject, cluster)
	if err != nil {
		return nil, err
	}

	secreteData, err := p.getSecreteData(resp, jobObject, cluster)
	if err != nil {
		return nil, err
	}

	return secreteData, nil
}

// GetAccessTokenForDrogon generates the token for the current user when using Drogon.
// Note that the returned token is only used for authentication.
// It will Not be used for data access in the spark job.
func (p Provider) GetAccessTokenForDrogon(
	ctx context.Context,
	jobObject runtime.Object,
	cluster *v2beta1pb.Cluster) (hadooptokenservice.Token, error) {

	resp, err := p.generateHadoopSecret(ctx, jobObject, cluster)
	if err != nil {
		return hadooptokenservice.Token{}, err
	}

	// At the moment, the SparkClient we are using only accept HDFS delegation Token. And
	// The token is only used for authentication of the Client. So we only return the HDFS
	// delegation token. It will Not be used for data access in the spark job.
	for _, generatedToken := range resp.Tokens {
		if generatedToken.ServiceType == _hdfsService {
			return generatedToken, nil
		}
	}

	return hadooptokenservice.Token{}, nil
}

func (p Provider) generateHadoopSecret(
	ctx context.Context,
	jobObject runtime.Object,
	cluster *v2beta1pb.Cluster) (*hadooptokenservice.GenerateTokensResponse, error) {

	region := cluster.Spec.GetRegion()
	alias := getAliasFromRegion(region)

	if alias == "" {
		return nil, fmt.Errorf("failed to get service alias for region:%s", region)
	}

	user, err := p.getJobUser(jobObject)
	if err != nil {
		return nil, fmt.Errorf("unable to get job's user name, %w", err)
	}

	var params []hadooptokenservice.GenerateTokenParams
	for serviceType, tokenKind := range _hadoopServiceTokenKindMap {
		params = append(params, hadooptokenservice.GenerateTokenParams{
			ServiceType:   serviceType,
			ServiceRegion: region,
			ServiceAlias:  alias,
			TokenKind:     tokenKind,
			User:          user,
		})
	}

	for serviceType, tokenKind := range _cloudFSTokenKindMap {
		params = append(params, hadooptokenservice.GenerateTokenParams{
			ServiceType:   serviceType,
			ServiceRegion: region,
			ServiceAlias:  _cloudLakeServiceAlias,
			TokenKind:     tokenKind,
			User:          user,
		})
	}

	// Add router config
	params = append(params, hadooptokenservice.GenerateTokenParams{
		ServiceType:   _hdfsService,
		ServiceRegion: region,
		ServiceAlias:  _routerServiceAlias,
		TokenKind:     _hadoopServiceTokenKindMap[_hdfsService],
		User:          user,
	})

	tokenResp, err := p.tokenGateway.GenerateTokens(
		ctx,
		&hadooptokenservice.GenerateTokensRequest{
			Params: params,
		})
	if err != nil {
		if errors.Is(err, hadooptokenservice.ErrPartialTokens) {
			return nil, err
		}
		return nil, fmt.Errorf("unable to get delegtaion tokens from token service, %w", err)
	}
	return tokenResp, nil
}

func (p Provider) getJobUser(jobObject runtime.Object) (string, error) {
	switch jobObject.(type) {
	case *v2beta1pb.RayJob:
		return jobObject.(*v2beta1pb.RayJob).Spec.User.Name, nil
	case *v2beta1pb.SparkJob:
		return jobObject.(*v2beta1pb.SparkJob).Spec.User.Name, nil
	default:
		return "", fmt.Errorf("invalid job type")
	}
}

func (p Provider) getSecreteData(
	resp *hadooptokenservice.GenerateTokensResponse,
	jobObject runtime.Object,
	cluster *v2beta1pb.Cluster) (map[string][]byte, error) {

	switch jobObject.(type) {
	case *v2beta1pb.RayJob:
		var (
			secureNS      = _defaultSecureNsPHX
			secureSchemes = []string{"hdfs"}
		)

		if strings.ToUpper(cluster.Spec.Region) == _dca {
			secureNS = _defaultSecureNsDCA
		}

		data := map[string]any{
			"token": resp.Credentials,
			"hdfs": map[string]any{
				"cluster":    cluster.Spec.Zone,
				"nameserver": secureNS,
				"schemes":    strings.Join(secureSchemes, ","),
			},
			"tokens": resp.Tokens,
		}
		dataBytes, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}

		return map[string][]byte{
			jobObject.(*v2beta1pb.RayJob).Name: dataBytes,
		}, nil

	case *v2beta1pb.SparkJob:
		decodedToken, err := base64.RawURLEncoding.DecodeString(resp.Credentials)
		if err != nil {
			return nil, fmt.Errorf("decode credential for spark job err:%v", err)
		}

		return map[string][]byte{
			_sparkSecretKey: decodedToken}, nil
	}

	return nil, fmt.Errorf("invalid job type")
}
