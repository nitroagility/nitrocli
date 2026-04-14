package core

import (
	"context"
	"fmt"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// AWSResolvedConn holds a resolved AWS connection's SDK config and produced credentials.
// Implements ResolvedConnection.
type AWSResolvedConn struct {
	cfg     awsv2.Config
	envVars map[string]string // AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN, AWS_REGION, etc.
}

// Vars returns the env vars produced by this connection (AWS_ACCESS_KEY_ID, etc.).
func (c *AWSResolvedConn) Vars() map[string]string { return c.envVars }

// SDKConfig returns the aws.Config for SDK clients.
func (c *AWSResolvedConn) SDKConfig() any { return c.cfg }

// resolveAWSConnection resolves an AWS connection for a specific environment.
//
// Resolution:
//   - auth[envName] with "assume-role": SDK default config → STS AssumeRole → temp creds
//   - auth[envName] with "static": read var values from vars map → static creds
//   - no auth entry: just region, rely on SDK default chain
func resolveAWSConnection(ctx context.Context, name string, conn *Connection, envName string, vars map[string]string) (*AWSResolvedConn, error) {
	auth := conn.Auth[envName]

	if auth == nil {
		// No auth: SDK default chain with region only.
		cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(conn.Region))
		if err != nil {
			return nil, fmt.Errorf("connection %q: failed to load AWS default config: %w", name, err)
		}
		return &AWSResolvedConn{
			cfg: cfg,
			envVars: map[string]string{
				"AWS_REGION":         conn.Region,
				"AWS_DEFAULT_REGION": conn.Region,
			},
		}, nil
	}

	switch auth.Method {
	case "assume-role":
		return resolveAssumeRole(ctx, name, conn.Region, auth)
	case "static":
		return resolveStaticAuth(ctx, name, conn.Region, auth, vars)
	default:
		return nil, fmt.Errorf("connection %q env %q: unsupported auth method %q", name, envName, auth.Method)
	}
}

func resolveAssumeRole(ctx context.Context, name, region string, auth *ConnectionAuth) (*AWSResolvedConn, error) {
	// Load SDK default config for bootstrap (env vars, profile, IMDS).
	baseCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("connection %q: failed to load base AWS config for assume-role: %w", name, err)
	}

	stsClient := sts.NewFromConfig(baseCfg)

	sessionName := auth.RoleSessionName
	if sessionName == "" {
		sessionName = "nitrocli-" + name
	}

	input := &sts.AssumeRoleInput{
		RoleArn:         &auth.RoleArn,
		RoleSessionName: &sessionName,
	}
	if auth.Duration > 0 {
		d := int32(auth.Duration)
		input.DurationSeconds = &d
	}

	result, err := stsClient.AssumeRole(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("connection %q: STS AssumeRole(%s) failed: %w", name, auth.RoleArn, err)
	}

	creds := result.Credentials

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				*creds.AccessKeyId,
				*creds.SecretAccessKey,
				*creds.SessionToken,
			),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("connection %q: failed to create config from assumed-role creds: %w", name, err)
	}

	return &AWSResolvedConn{
		cfg: cfg,
		envVars: map[string]string{
			"AWS_ACCESS_KEY_ID":     *creds.AccessKeyId,
			"AWS_SECRET_ACCESS_KEY": *creds.SecretAccessKey,
			"AWS_SESSION_TOKEN":     *creds.SessionToken,
			"AWS_REGION":            region,
			"AWS_DEFAULT_REGION":    region,
		},
	}, nil
}

func resolveStaticAuth(ctx context.Context, name, region string, auth *ConnectionAuth, vars map[string]string) (*AWSResolvedConn, error) {
	ak, okAK := vars[auth.AccessKeyIDVar]
	sk, okSK := vars[auth.SecretAccessKeyVar]
	if !okAK || ak == "" {
		return nil, fmt.Errorf("connection %q: static auth: variable %q not resolved (must come from globals)", name, auth.AccessKeyIDVar)
	}
	if !okSK || sk == "" {
		return nil, fmt.Errorf("connection %q: static auth: variable %q not resolved (must come from globals)", name, auth.SecretAccessKeyVar)
	}

	sessionToken := ""
	if auth.SessionTokenVar != "" {
		if tok, ok := vars[auth.SessionTokenVar]; ok {
			sessionToken = tok
		}
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(ak, sk, sessionToken),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("connection %q: failed to create config from static creds: %w", name, err)
	}

	resolved := &AWSResolvedConn{
		cfg: cfg,
		envVars: map[string]string{
			"AWS_ACCESS_KEY_ID":     ak,
			"AWS_SECRET_ACCESS_KEY": sk,
			"AWS_REGION":            region,
			"AWS_DEFAULT_REGION":    region,
		},
	}
	if sessionToken != "" {
		resolved.envVars["AWS_SESSION_TOKEN"] = sessionToken
	}
	return resolved, nil
}

// newAWSResolverFromConfig creates a Secrets Manager resolver using a connection's SDK config.
func newAWSResolverFromConfig(cfg awsv2.Config) *awsResolver {
	return &awsResolver{
		client: secretsmanager.NewFromConfig(cfg),
		cache:  make(map[string]string),
	}
}
