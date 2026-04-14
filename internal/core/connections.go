package core

import (
	"context"
	"fmt"
)

// ResolvedConnection is implemented by every cloud-specific resolved connection.
// It provides a uniform way to read exported env vars and retrieve the
// underlying SDK config via type assertion.
type ResolvedConnection interface {
	// Vars returns the env vars this connection produces (e.g. AWS_ACCESS_KEY_ID).
	Vars() map[string]string
	// SDKConfig returns the cloud-specific SDK config. Callers type-assert to
	// the concrete type they need (e.g. *AWSResolvedConn for AWS).
	SDKConfig() any
}

// ConnectionResolver resolves named connections and caches their SDK clients.
// Called AFTER loadGlobals and BEFORE provider resolution.
//
// The resolvedClients map stores cloud-agnostic ResolvedConnection values.
// Provider resolvers type-assert to the concrete type (e.g. AWSResolvedConn)
// when they need the SDK client.
type ConnectionResolver struct {
	Log    *Logger
	Masker *Masker

	// resolvedClients keyed by connection name.
	resolvedClients map[string]ResolvedConnection
}

// Resolve iterates all connections applicable to envName.
// For exportEnv connections, credentials are injected into the vars map.
// All resolved SDK configs are cached for provider/command consumption.
func (cr *ConnectionResolver) Resolve(ctx context.Context, connections map[string]*Connection, envName string, vars map[string]string) error {
	if len(connections) == 0 {
		return nil
	}

	cr.resolvedClients = make(map[string]ResolvedConnection, len(connections))

	for name, conn := range connections {
		if !connectionAppliesToEnv(conn, envName) {
			continue
		}

		cr.Log.Info(fmt.Sprintf("connection %q (type: %s)", name, conn.Type))

		switch conn.Type {
		case "aws":
			resolved, err := resolveAWSConnection(ctx, name, conn, envName, vars)
			if err != nil {
				return err
			}
			cr.resolvedClients[name] = resolved
			cr.maskSecrets(resolved.Vars())

		default:
			return fmt.Errorf("connection %q: unsupported type %q", name, conn.Type)
		}

		// Export to shared vars if exportEnv is true.
		resolved := cr.resolvedClients[name]
		if conn.IsExportEnv() {
			for k, v := range resolved.Vars() {
				vars[k] = v
			}
			cr.Log.Info(fmt.Sprintf("  exported %d env vars (exportEnv: true)", len(resolved.Vars())))
		} else {
			cr.Log.Info("  resolved (SDK client cached, no env export)")
		}
	}

	return nil
}

// Get returns the resolved connection by name. Returns nil if not found.
func (cr *ConnectionResolver) Get(name string) ResolvedConnection {
	if cr.resolvedClients == nil {
		return nil
	}
	return cr.resolvedClients[name]
}

// AWSConfig returns the resolved AWS connection, or nil if the named
// connection is not AWS or was not resolved.
func (cr *ConnectionResolver) AWSConfig(name string) *AWSResolvedConn {
	conn := cr.Get(name)
	if conn == nil {
		return nil
	}
	aws, _ := conn.(*AWSResolvedConn)
	return aws
}

// ConnectionVars returns the credential variables for a named connection.
// Used by the executor to inject connection-specific env vars for commands.
func (cr *ConnectionResolver) ConnectionVars(name string) map[string]string {
	conn := cr.Get(name)
	if conn == nil {
		return nil
	}
	return conn.Vars()
}

// maskSecrets registers known secret var names in the masker.
func (cr *ConnectionResolver) maskSecrets(vars map[string]string) {
	// AWS
	for _, key := range []string{"AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"} {
		if v, ok := vars[key]; ok && v != "" {
			cr.Masker.Add(v)
		}
	}
	// Azure (future)
	for _, key := range []string{"AZURE_CLIENT_SECRET", "ARM_CLIENT_SECRET"} {
		if v, ok := vars[key]; ok && v != "" {
			cr.Masker.Add(v)
		}
	}
	// GCP (future)
	for _, key := range []string{"GOOGLE_APPLICATION_CREDENTIALS_JSON"} {
		if v, ok := vars[key]; ok && v != "" {
			cr.Masker.Add(v)
		}
	}
}

func connectionAppliesToEnv(conn *Connection, envName string) bool {
	if len(conn.Envs) == 0 {
		return true
	}
	for _, e := range conn.Envs {
		if e == envName {
			return true
		}
	}
	return false
}
