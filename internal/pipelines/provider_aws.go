package pipelines

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
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

// newAWSResolver creates an AWS Secrets Manager client for the given region.
// Credentials are resolved via the standard AWS SDK chain:
//
//	env vars → shared config → IAM role → EC2 IMDS
func newAWSResolver(ctx context.Context, region string) (*awsResolver, error) {
	opts := []func(*awsconfig.LoadOptions) error{}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
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

	// Extract a single key from a JSON secret.
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return "", fmt.Errorf("secret %q is not valid JSON (needed to extract key %q): %w", v.Path, v.Key, err)
	}

	val, ok := m[v.Key]
	if !ok {
		return "", fmt.Errorf("secret %q does not contain key %q", v.Path, v.Key)
	}

	// Return the value as a string regardless of JSON type.
	switch tv := val.(type) {
	case string:
		return tv, nil
	default:
		b, _ := json.Marshal(tv)
		return string(b), nil
	}
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
