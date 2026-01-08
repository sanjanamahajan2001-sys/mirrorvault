package detect

import (
	"os"
	"strings"

	"mirrorvault/pkg/model"
)

func DetectRedis() *model.Database {
	if !CommandExists("redis-cli") {
		return nil
	}

	versionRaw, _ := execOutput("redis-server", "--version")

	requiresAuth := false
	conf, err := os.ReadFile("/etc/redis/redis.conf")
	if err == nil && strings.Contains(string(conf), "requirepass") {
		requiresAuth = true
	}

	return &model.Database{
		Engine:       "Redis",
		Version:      normalizeVersion(versionRaw),
		Type:         model.NoSQL,
		RequiresAuth: requiresAuth,
		Names:        []string{"dump.rdb"},
	}
}
