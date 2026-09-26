package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sqlrs/cli/internal/authsession"
	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/util"
)

type authLoginOptions = authsession.LoginOptions
type authLoginResult = authsession.LoginResult
type authStatusOptions = authsession.StatusOptions
type authStatusResult = authsession.StatusResult
type authLogoutOptions = authsession.LogoutOptions
type authLogoutResult = authsession.LogoutResult
type authResolveOptions = authsession.ResolveOptions
type authResolvedBearerToken = authsession.ResolvedBearerToken

type authManager interface {
	LoginGoogle(context.Context, authLoginOptions) (authLoginResult, error)
	Status(context.Context, authStatusOptions) (authStatusResult, error)
	Logout(context.Context, authLogoutOptions) (authLogoutResult, error)
	ResolveBearerToken(context.Context, authResolveOptions) (authResolvedBearerToken, error)
}

var authManagerFactory = func() authManager {
	return authsession.NewManager(authsession.ManagerOptions{})
}

var authProviderLoader = func(ctx context.Context, endpoint, provider string) (client.AuthProviderConfiguration, error) {
	return client.New(endpoint, client.Options{}).GetAuthProvider(ctx, provider)
}

var authProviderIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

type authInvocation struct {
	action    string
	provider  string
	loginHint string
	noBrowser bool
	noRevoke  bool
}

func parseAuthArgs(args []string) (authInvocation, bool, error) {
	var invocation authInvocation
	if err := validateNoUnicodeDashFlags(args, 2); err != nil {
		return invocation, false, err
	}
	if len(args) == 0 {
		return invocation, false, ExitErrorf(2, "Missing auth command")
	}
	action := strings.TrimSpace(args[0])
	if action == "--help" || action == "-h" {
		return invocation, true, nil
	}
	switch action {
	case "login":
		return parseAuthLoginArgs(args[1:])
	case "status":
		if hasHelpArg(args[1:]) {
			return invocation, true, nil
		}
		if len(args) > 1 {
			return invocation, false, ExitErrorf(2, "auth status does not accept arguments")
		}
		invocation.action = "status"
		return invocation, false, nil
	case "logout":
		return parseAuthLogoutArgs(args[1:])
	default:
		return invocation, false, ExitErrorf(2, "Unknown auth command: %s", action)
	}
}

func parseAuthLoginArgs(args []string) (authInvocation, bool, error) {
	invocation := authInvocation{action: "login"}
	if len(args) == 0 {
		return invocation, false, ExitErrorf(2, "auth login provider is required")
	}
	provider := strings.TrimSpace(args[0])
	if provider == "--help" || provider == "-h" {
		return invocation, true, nil
	}
	if !authProviderIDPattern.MatchString(provider) {
		return invocation, false, ExitErrorf(2, "invalid auth provider: %s", provider)
	}
	fs := flag.NewFlagSet("sqlrs auth login <provider>", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	loginHint := fs.String("login-hint", "", "Google account email hint")
	noBrowser := fs.Bool("no-browser", false, "print URL instead of opening browser")
	help := fs.Bool("help", false, "show help")
	helpShort := fs.Bool("h", false, "show help")
	if err := fs.Parse(args[1:]); err != nil {
		return invocation, false, ExitErrorf(2, "Invalid arguments: %v", err)
	}
	if *help || *helpShort {
		return invocation, true, nil
	}
	if fs.NArg() > 0 {
		return invocation, false, ExitErrorf(2, "auth login does not accept positional arguments after the provider")
	}
	invocation.provider = provider
	invocation.loginHint = strings.TrimSpace(*loginHint)
	invocation.noBrowser = *noBrowser
	return invocation, false, nil
}

func parseAuthLogoutArgs(args []string) (authInvocation, bool, error) {
	invocation := authInvocation{action: "logout", provider: "google"}
	fs := flag.NewFlagSet("sqlrs auth logout", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	noRevoke := fs.Bool("no-revoke", false, "delete local credentials without revoking Google refresh token")
	help := fs.Bool("help", false, "show help")
	helpShort := fs.Bool("h", false, "show help")
	if err := fs.Parse(args); err != nil {
		return invocation, false, ExitErrorf(2, "Invalid arguments: %v", err)
	}
	if *help || *helpShort {
		return invocation, true, nil
	}
	if fs.NArg() > 0 {
		return invocation, false, ExitErrorf(2, "auth logout does not accept positional arguments")
	}
	invocation.noRevoke = *noRevoke
	return invocation, false, nil
}

func runAuth(stdout, stderr io.Writer, cwd string, opts cli.GlobalOptions, args []string) error {
	invocation, showHelp, err := parseAuthArgs(args)
	if err != nil {
		return err
	}
	if showHelp {
		printAuthUsage(stdout)
		return nil
	}
	cmdCtx, err := resolveCommandContext(cwd, opts)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cmdCtx.mode) != "remote" {
		return fmt.Errorf("auth commands require remote mode")
	}
	if strings.TrimSpace(cmdCtx.profile.Endpoint) == "" || strings.TrimSpace(cmdCtx.profile.Endpoint) == "auto" {
		return fmt.Errorf("auth commands require an explicit remote endpoint")
	}
	if strings.EqualFold(strings.TrimSpace(cmdCtx.profile.Auth.Mode), "oidcSession") {
		fmt.Fprintln(stderr, "warning: auth.mode oidcSession is deprecated; run `sqlrs init remote <endpoint> --update`")
	}

	manager := authManagerFactory()
	switch invocation.action {
	case "login":
		authMode := strings.TrimSpace(cmdCtx.profile.Auth.Mode)
		if !strings.EqualFold(authMode, "oidcSession") && !strings.EqualFold(authMode, "remoteSession") {
			return fmt.Errorf("auth login requires profile auth.mode: remoteSession")
		}
		providerConfig := client.AuthProviderConfiguration{
			ID: "google", Adapter: "oidc", ConfigurationVersion: 1,
			Issuer: cmdCtx.profile.Auth.Issuer, ClientID: cmdCtx.profile.Auth.ClientID,
		}
		if strings.EqualFold(authMode, "remoteSession") {
			providerConfig, err = authProviderLoader(context.Background(), cmdCtx.profile.Endpoint, invocation.provider)
			if err != nil {
				return fmt.Errorf("discover auth provider %q: %w", invocation.provider, err)
			}
		}
		var authorizationURLReady func(string) error
		if invocation.noBrowser {
			authorizationURLReady = func(authURL string) error {
				_, err := fmt.Fprintf(stderr, "authorizationURL: %s\n", authURL)
				return err
			}
		}
		result, err := manager.LoginGoogle(context.Background(), authLoginOptions{
			ProfileName:             cmdCtx.profileName,
			Endpoint:                cmdCtx.profile.Endpoint,
			InstallationID:          cmdCtx.profile.InstallationID,
			ControlEndpoint:         cmdCtx.profile.InstallationEndpoint,
			Provider:                invocation.provider,
			ClientID:                providerConfig.ClientID,
			Issuer:                  providerConfig.Issuer,
			LoginHint:               invocation.loginHint,
			NoBrowser:               invocation.noBrowser,
			AuthorizationURLReady:   authorizationURLReady,
			AuthorizationEndpoint:   providerConfig.AuthorizationEndpoint,
			TokenEndpoint:           providerConfig.TokenEndpoint,
			Scopes:                  providerConfig.Scopes,
			AuthorizationParameters: providerConfig.AuthorizationParameters,
			RequireCallbackIssuer:   strings.EqualFold(authMode, "remoteSession"),
		})
		if err != nil {
			return err
		}
		result.AuthorizationURL = ""
		reconcileErr := reconcileLoginEndpoint(context.Background(), &cmdCtx, result.BearerToken, stderr)
		result.Endpoint = cmdCtx.profile.Endpoint
		if err := writeAuthLoginResult(stdout, result, cmdCtx.output); err != nil {
			return err
		}
		if reconcileErr != nil {
			fmt.Fprintf(stderr, "warning: login succeeded, but endpoint reconciliation failed: %v\n", reconcileErr)
			return ExitErrorf(1, "login succeeded; endpoint reconciliation requires recovery")
		}
		return nil
	case "status":
		result, err := manager.Status(context.Background(), authStatusOptions{
			ProfileName:     cmdCtx.profileName,
			Endpoint:        cmdCtx.profile.Endpoint,
			InstallationID:  cmdCtx.profile.InstallationID,
			ControlEndpoint: cmdCtx.profile.InstallationEndpoint,
			AuthMode:        cmdCtx.profile.Auth.Mode,
			ClientID:        cmdCtx.profile.Auth.ClientID,
			Issuer:          cmdCtx.profile.Auth.Issuer,
			TokenEnv:        cmdCtx.profile.Auth.TokenEnv,
		})
		if err != nil {
			return err
		}
		return writeAuthStatusResult(stdout, result, cmdCtx.output)
	case "logout":
		var providerResolver func(context.Context, string) (authsession.ProviderConfiguration, error)
		if strings.EqualFold(strings.TrimSpace(cmdCtx.profile.Auth.Mode), "remoteSession") {
			providerResolver = func(ctx context.Context, provider string) (authsession.ProviderConfiguration, error) {
				detail, err := authProviderLoader(ctx, cmdCtx.profile.Endpoint, provider)
				if err != nil {
					return authsession.ProviderConfiguration{}, err
				}
				return authsession.ProviderConfiguration{ID: detail.ID, Issuer: detail.Issuer, ClientID: detail.ClientID, TokenEndpoint: detail.TokenEndpoint, RevocationEndpoint: detail.RevocationEndpoint}, nil
			}
		}
		result, err := manager.Logout(context.Background(), authLogoutOptions{
			ProfileName:      cmdCtx.profileName,
			Endpoint:         cmdCtx.profile.Endpoint,
			InstallationID:   cmdCtx.profile.InstallationID,
			ControlEndpoint:  cmdCtx.profile.InstallationEndpoint,
			ClientID:         cmdCtx.profile.Auth.ClientID,
			Issuer:           cmdCtx.profile.Auth.Issuer,
			NoRevoke:         invocation.noRevoke,
			ProviderResolver: providerResolver,
		})
		if err != nil {
			return err
		}
		return writeAuthLogoutResult(stdout, result, cmdCtx.output)
	default:
		return fmt.Errorf("unknown auth command: %s", invocation.action)
	}
}

func reconcileLoginEndpoint(ctx context.Context, cmdCtx *commandContext, token string, stderr io.Writer) error {
	if strings.TrimSpace(token) == "" || !strings.EqualFold(strings.TrimSpace(cmdCtx.profile.Auth.Mode), "remoteSession") {
		return nil
	}
	control := normalizeRemoteEndpoint(cmdCtx.profile.InstallationEndpoint)
	current := normalizeRemoteEndpoint(cmdCtx.profile.Endpoint)
	if control == "" {
		return fmt.Errorf("profile installationEndpoint is missing")
	}
	api := client.New(current, client.Options{AuthToken: token, Timeout: cmdCtx.timeout})
	me, found, err := api.GetCurrentUser(ctx)
	if err != nil {
		return err
	}
	if !found {
		if current == control {
			fmt.Fprintln(stderr, "authentication succeeded; register this identity with `sqlrs user register`")
			return nil
		}
		return fmt.Errorf("organization endpoint is not visible; rerun `sqlrs init remote %s --update`", control)
	}
	if current != control {
		for _, membership := range me.Memberships {
			if normalizeRemoteEndpoint(membership.Organization.Endpoint) == current {
				return nil
			}
		}
		return fmt.Errorf("authenticated organization does not match candidate endpoint; rerun `sqlrs init remote %s --update`", control)
	}
	if len(me.Memberships) != 1 {
		return nil
	}
	organization := me.Memberships[0].Organization
	canonical := normalizeRemoteEndpoint(organization.Endpoint)
	if canonical == "" {
		return fmt.Errorf("organization %q did not provide a canonical endpoint", organization.Slug)
	}
	if err := client.ValidateConnectionInfo(client.ConnectionInfo{InstallationID: cmdCtx.profile.InstallationID, Endpoints: client.ConnectionEndpoints{Control: control, Current: canonical}}); err != nil {
		return fmt.Errorf("untrusted canonical organization endpoint: %w", err)
	}
	if err := persistSelectedProfileEndpoint(*cmdCtx, canonical); err != nil {
		return err
	}
	old := cmdCtx.profile.Endpoint
	cmdCtx.profile.Endpoint = canonical
	fmt.Fprintf(stderr, "warning: profile %q switched to organization %q: %s -> %s\n", cmdCtx.profileName, organization.Slug, old, canonical)
	return nil
}

func persistSelectedProfileEndpoint(cmdCtx commandContext, endpoint string) error {
	path := strings.TrimSpace(cmdCtx.cfgResult.ProjectConfigPath)
	if path == "" {
		return fmt.Errorf("workspace config path is unavailable")
	}
	raw, err := readConfigMap(path)
	if err != nil {
		return err
	}
	setNested(raw, []string{"profiles", cmdCtx.profileName, "endpoint"}, endpoint)
	data, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	return util.AtomicWriteFile(path, data, 0o600)
}

func resolveEffectiveAuthToken(ctx context.Context, cmdCtx commandContext) (commandContext, error) {
	if strings.TrimSpace(cmdCtx.mode) != "remote" {
		return cmdCtx, nil
	}
	if token := resolveAuthToken(cmdCtx.profile.Auth); token != "" {
		cmdCtx.authToken = token
		if isAppSessionMode(cmdCtx.profile.Auth.Mode) {
			cmdCtx.authTokenSource = authsession.TokenSourceEnvironmentOverride
		} else {
			cmdCtx.authTokenSource = authsession.TokenSourceLegacyBearer
		}
		return cmdCtx, nil
	}
	mode := strings.TrimSpace(cmdCtx.profile.Auth.Mode)
	if !strings.EqualFold(mode, "oidcSession") && !strings.EqualFold(mode, "remoteSession") {
		return cmdCtx, nil
	}
	resolved, err := authManagerFactory().ResolveBearerToken(ctx, authResolveOptions{
		ProfileName:     cmdCtx.profileName,
		Endpoint:        cmdCtx.profile.Endpoint,
		AuthMode:        cmdCtx.profile.Auth.Mode,
		ClientID:        cmdCtx.profile.Auth.ClientID,
		ClientSecret:    cmdCtx.profile.Auth.ClientSecret,
		Issuer:          cmdCtx.profile.Auth.Issuer,
		TokenEnv:        cmdCtx.profile.Auth.TokenEnv,
		StaticToken:     cmdCtx.profile.Auth.Token,
		InstallationID:  cmdCtx.profile.InstallationID,
		ControlEndpoint: cmdCtx.profile.InstallationEndpoint,
		ProviderResolver: func(ctx context.Context, provider string) (authsession.ProviderConfiguration, error) {
			detail, err := authProviderLoader(ctx, cmdCtx.profile.Endpoint, provider)
			if err != nil {
				return authsession.ProviderConfiguration{}, err
			}
			return authsession.ProviderConfiguration{ID: detail.ID, Issuer: detail.Issuer, ClientID: detail.ClientID, TokenEndpoint: detail.TokenEndpoint}, nil
		},
	})
	if err != nil {
		return cmdCtx, err
	}
	cmdCtx.authToken = strings.TrimSpace(resolved.Token)
	cmdCtx.authTokenSource = resolved.Source
	return cmdCtx, nil
}

func isAppSessionMode(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), "oidcSession") || strings.EqualFold(strings.TrimSpace(mode), "remoteSession")
}

func writeAuthLoginResult(w io.Writer, result authLoginResult, output string) error {
	result.AuthorizationURL = ""
	if output == "json" {
		return writeJSON(w, result)
	}
	fmt.Fprintln(w, "logged in")
	printAuthMetadata(w, result.Provider, result.Email, result.Issuer, result.Audience, result.TokenExpiry, result.Profile, result.Endpoint, "")
	return nil
}

func writeAuthStatusResult(w io.Writer, result authStatusResult, output string) error {
	if output == "json" {
		return writeJSON(w, result)
	}
	if result.LoggedIn {
		fmt.Fprintln(w, "status: logged in")
	} else {
		fmt.Fprintln(w, "status: not logged in")
	}
	printAuthMetadata(w, result.Provider, result.Email, result.Issuer, result.Audience, result.TokenExpiry, result.Profile, result.Endpoint, result.Override)
	return nil
}

func writeAuthLogoutResult(w io.Writer, result authLogoutResult, output string) error {
	if output == "json" {
		return writeJSON(w, result)
	}
	fmt.Fprintln(w, "logged out")
	if result.Provider != "" {
		fmt.Fprintf(w, "provider: %s\n", result.Provider)
	}
	if result.Profile != "" {
		fmt.Fprintf(w, "profile: %s\n", result.Profile)
	}
	if result.Endpoint != "" {
		fmt.Fprintf(w, "endpoint: %s\n", result.Endpoint)
	}
	fmt.Fprintf(w, "revoked: %t\n", result.Revoked)
	if result.RevocationFailed != "" {
		fmt.Fprintf(w, "revocationWarning: %s\n", result.RevocationFailed)
	}
	return nil
}

func printAuthMetadata(w io.Writer, provider, email, issuer, audience string, tokenExpiry interface {
	IsZero() bool
	Format(string) string
}, profile, endpoint, override string) {
	if provider != "" {
		fmt.Fprintf(w, "provider: %s\n", provider)
	}
	if email != "" {
		fmt.Fprintf(w, "email: %s\n", email)
	}
	if issuer != "" {
		fmt.Fprintf(w, "issuer: %s\n", issuer)
	}
	if audience != "" {
		fmt.Fprintf(w, "audience: %s\n", audience)
	}
	if !tokenExpiry.IsZero() {
		fmt.Fprintf(w, "tokenExpiry: %s\n", tokenExpiry.Format(timeFormatRFC3339))
	}
	if profile != "" {
		fmt.Fprintf(w, "profile: %s\n", profile)
	}
	if endpoint != "" {
		fmt.Fprintf(w, "endpoint: %s\n", endpoint)
	}
	if override != "" {
		fmt.Fprintf(w, "override: %s\n", override)
	} else {
		fmt.Fprintln(w, "override: none")
	}
}

const timeFormatRFC3339 = "2006-01-02T15:04:05Z07:00"

func printAuthUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  sqlrs auth login google [--login-hint <email>] [--no-browser]")
	fmt.Fprintln(w, "  sqlrs auth status")
	fmt.Fprintln(w, "  sqlrs auth logout [--no-revoke]")
}
