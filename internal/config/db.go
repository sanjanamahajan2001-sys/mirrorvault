package config

import (
	"os"
	"strings"
)

func MySQLUser() string {
	if v := os.Getenv("MV_MYSQL_USER"); v != "" {
		return v
	}
	return "root"
}

func MySQLHost() string {
	return os.Getenv("MV_MYSQL_HOST")
}

func MySQLPort() string {
	return os.Getenv("MV_MYSQL_PORT")
}

func PostgresUser() string {
	if v := os.Getenv("MV_PG_USER"); v != "" {
		return v
	}
	return "postgres"
}

func PostgresHost() string {
	return os.Getenv("MV_PG_HOST")
}

func PostgresPort() string {
	return os.Getenv("MV_PG_PORT")
}

func MongoUser() string {
	if v := os.Getenv("MV_MONGO_USER"); v != "" {
		return v
	}
	return "admin"
}

func MongoAuthDB() string {
	if v := os.Getenv("MV_MONGO_AUTHDB"); v != "" {
		return v
	}
	return "admin"
}

func MongoURI() string {
	return os.Getenv("MV_MONGO_URI")
}

func MSSQLUser() string {
	if v := os.Getenv("MV_MSSQL_USER"); v != "" {
		return v
	}
	return "sa"
}

func MSSQLServer() string {
	host := os.Getenv("MV_MSSQL_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("MV_MSSQL_PORT")
	if port == "" {
		return host
	}
	return strings.Join([]string{host, port}, ",")
}
