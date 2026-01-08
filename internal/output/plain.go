package output

import (
	"fmt"
	"mirrorvault/pkg/model"
)

func PrintScanResult(result model.ScanResult) {
	fmt.Println("\nScanning server for databases:\n")

	var sqlPrinted, nosqlPrinted bool

	for _, db := range result.Databases {
		if db.Type == model.SQL && !sqlPrinted {
			fmt.Println("SQL Databases")
			sqlPrinted = true
		}
		if db.Type == model.NoSQL && !nosqlPrinted {
			fmt.Println("\nNoSQL Databases")
			nosqlPrinted = true
		}

		auth := "no auth"
		if db.RequiresAuth {
			auth = "auth required"
		}

		fmt.Printf(" %d) %s (%s) [%s]\n", db.ID, db.Engine, db.Version, auth)
		for _, name := range db.Names {
			fmt.Printf("    - %s\n", name)
		}
	}
}
