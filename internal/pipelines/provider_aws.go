package pipelines

import (
	"context"
	"fmt"
	"sync"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// awsResolver resolves variables from AWS Secrets Manager.
// It caches raw secret values so multiple variables referencing the same
// secret path (with different keys) only trigger one API call.
type awsResolver struct {
	client *secretsmanager.Client
	cache  map[string]string
	mu     sync.Mutex
}

// awsStaticCreds holds credentials extracted from already-resolved variables.
// When passed to newAWSResolver, they override the SDK default credentials chain.
type awsStaticCreds struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string // optional, used for temporary credentials
}

// newAWSResolver creates an AWS Secrets Manager client for the given region.
// If creds is non-nil, its values are used directly (static credentials provider).
// Otherwise the SDK default chain is used:
//
//	env vars → shared config → IAM role → EC2 IMDS
func newAWSResolver(ctx context.Context, region string, creds *awsStaticCreds) (*awsResolver, error) {
	opts := []func(*awsconfig.LoadOptions) error{}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	if creds != nil {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken),
		))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &awsResolver{
		client: secretsmanager.NewFromConfig(cfg),
		cache:  make(map[string]string),
	}, nil
}

// resolve fetches the secret value from AWS Secrets Manager.
// If v.Key is set, the raw value is parsed as JSON and only that key is returned.
// If v.Key is empty, the entire raw secret string is returned.
func (a *awsResolver) resolve(ctx context.Context, v *Variable) (string, error) {
	raw, err := a.getRaw(ctx, v.Path)
	if err != nil {
		return "", err
	}

	if v.Key == "" {
		return raw, nil
	}

	return extractJSONKey(raw, v.Path, v.Key)
}

func (a *awsResolver) getRaw(ctx context.Context, path string) (string, error) {
	a.mu.Lock()
	if v, ok := a.cache[path]; ok {
		a.mu.Unlock()
		return v, nil
	}
	a.mu.Unlock()

	out, err := a.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &path,
	})
	if err != nil {
		return "", fmt.Errorf("GetSecretValue(%q): %w", path, err)
	}

	if out.SecretString == nil {
		return "", fmt.Errorf("secret %q has no string value (binary secrets are not supported)", path)
	}

	a.mu.Lock()
	a.cache[path] = *out.SecretString
	a.mu.Unlock()

	return *out.SecretString, nil
}
