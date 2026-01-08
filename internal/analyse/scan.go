package analyse

import (
	"mirrorvault/internal/analyse/detect"
	"mirrorvault/pkg/model"
)

func ScanDatabases() model.ScanResult {
	var dbs []model.Database

	if db := detect.DetectMySQL(); db != nil {
		dbs = append(dbs, *db)
	}
	if db := detect.DetectPostgres(); db != nil {
		dbs = append(dbs, *db)
	}
	if db := detect.DetectSQLite(); db != nil {
		dbs = append(dbs, *db)
	}
	if db := detect.DetectMongoDB(); db != nil {
		dbs = append(dbs, *db)
	}
	if db := detect.DetectRedis(); db != nil {
		dbs = append(dbs, *db)
	}
	if db := detect.DetectMSSQL(); db != nil {
		dbs = append(dbs, *db)
	}

	return model.ScanResult{Databases: dbs}
}
