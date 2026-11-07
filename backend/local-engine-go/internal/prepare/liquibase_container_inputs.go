package prepare

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

// prepareContainerLiquibaseInputs retains ordered directory views and logical
// changelog names. It is the initial nonempty local search-path adapter, not the
// expanded modes/defaults/ref grammar in #173. Source bytes are captured later,
// before admission; none of these host paths enter executable settings.
func prepareContainerLiquibaseInputs(req Request) (preparedRequest, error) {
	cwd := req.WorkDir
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return preparedRequest{}, err
	}
	p, err := prepareLiquibaseArgs(req.LiquibaseArgs, abs, false, false)
	if err != nil {
		return preparedRequest{}, err
	}
	roots := p.searchPaths
	if len(roots) == 0 {
		roots = []string{abs}
	}
	var mounts []engineRuntime.Mount
	for _, root := range roots {
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			return preparedRequest{}, ValidationError{Code: "invalid_argument", Message: "Liquibase search root is unavailable"}
		}
		mounts = append(mounts, engineRuntime.Mount{HostPath: filepath.Clean(root), ContainerPath: fmt.Sprintf("/sqlrs/mnt/path%d", len(mounts)+1), ReadOnly: true})
	}
	project := func(value string, logical bool) (string, error) {
		if looksLikeRemoteRef(value) {
			return value, nil
		}
		for _, mount := range mounts {
			rel, err := filepath.Rel(mount.HostPath, value)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
				if logical {
					return filepath.ToSlash(rel), nil
				}
				if rel == "." {
					return mount.ContainerPath, nil
				}
				return mount.ContainerPath + "/" + filepath.ToSlash(rel), nil
			}
		}
		return "", ValidationError{Code: "invalid_argument", Message: "Liquibase file is outside admitted search roots"}
	}
	args := append([]string(nil), p.normalizedArgs...)
	hasSearch := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		flag, value, equals := strings.Cut(arg, "=")
		if flag != "--changelog-file" && flag != "--defaults-file" && flag != "--searchPath" && flag != "--search-path" {
			continue
		}
		if !equals {
			i++
			value = args[i]
		}
		if flag == "--searchPath" || flag == "--search-path" {
			hasSearch = true
			flag = "--search-path"
			parts := strings.Split(value, ",")
			for n, item := range parts {
				parts[n], err = project(item, false)
				if err != nil {
					return preparedRequest{}, err
				}
			}
			value = strings.Join(parts, ",")
		} else {
			value, err = project(value, flag == "--changelog-file")
			if err != nil {
				return preparedRequest{}, err
			}
		}
		if equals {
			args[i] = flag + "=" + value
		} else {
			args[i-1], args[i] = flag, value
		}
	}
	if !hasSearch {
		mapped := make([]string, len(mounts))
		for i, mount := range mounts {
			mapped[i] = mount.ContainerPath
		}
		args = append([]string{"--search-path=" + strings.Join(mapped, ",")}, args...)
	}
	return preparedRequest{request: req, normalizedArgs: args, argsNormalized: strings.Join(args, " "), liquibaseMounts: mounts, liquibaseContainerWorkDir: mounts[0].ContainerPath, liquibaseWorkDir: abs}, nil
}
