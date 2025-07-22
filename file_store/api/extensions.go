package api

import (
	"fmt"
	"strings"
)

func GetExtensionForDatastore(path_spec DSPathSpec) string {
	t := path_spec.Type()

	switch t {
	case PATH_TYPE_DATASTORE_PROTO:
		return ".db"

	case PATH_TYPE_DATASTORE_JSON:
		return ".json.db"

	case PATH_TYPE_DATASTORE_DIRECTORY:
		return ""

	}
	return ".db"
}

func GetExtensionForFilestore(path_spec FSPathSpec) string {
	t := path_spec.Type()

	switch t {
	case PATH_TYPE_DATASTORE_PROTO, PATH_TYPE_DATASTORE_JSON:
		panic(fmt.Sprintf(
			"datastore path used for filestore for %v ",
			path_spec.Components()))

	case PATH_TYPE_FILESTORE_JSON:
		return ".json"

	case PATH_TYPE_FILESTORE_JSON_INDEX:
		return ".json.index"

	case PATH_TYPE_FILESTORE_JSON_TIME_INDEX:
		return ".json.tidx"

	case PATH_TYPE_FILESTORE_SPARSE_IDX:
		return ".idx"

	case PATH_TYPE_FILESTORE_DOWNLOAD_ZIP:
		return ".zip"

	case PATH_TYPE_FILESTORE_CHUNK_INDEX:
		return ".chunk"

	case PATH_TYPE_FILESTORE_DOWNLOAD_REPORT:
		return ".html"

	case PATH_TYPE_FILESTORE_TMP:
		return ".tmp"

	case PATH_TYPE_FILESTORE_CSV:
		return ".csv"

	case PATH_TYPE_FILESTORE_YAML:
		return ".yaml"

	case PATH_TYPE_FILESTORE_DB:
		return ".db"

	case PATH_TYPE_FILESTORE_DB_JSON:
		return ".json.db"

	case PATH_TYPE_FILESTORE_ANY:
		return ""
	}

	return ""
}

func GetDataStorePathTypeFromExtension(name string) (PathType, string) {
	if strings.HasSuffix(name, ".json.db") {
		return PATH_TYPE_DATASTORE_JSON, name[:len(name)-8]
	}

	if strings.HasSuffix(name, ".db") {
		return PATH_TYPE_DATASTORE_PROTO, name[:len(name)-3]
	}

	return PATH_TYPE_DATASTORE_UNKNOWN, name
}

func GetFileStorePathTypeFromExtension(name string) (PathType, string) {
	if strings.HasSuffix(name, ".json") {
		return PATH_TYPE_FILESTORE_JSON, name[:len(name)-5]
	}

	if strings.HasSuffix(name, ".json.index") {
		return PATH_TYPE_FILESTORE_JSON_INDEX, name[:len(name)-11]
	}

	if strings.HasSuffix(name, ".json.tidx") {
		return PATH_TYPE_FILESTORE_JSON_TIME_INDEX, name[:len(name)-10]
	}

	if strings.HasSuffix(name, ".json.db") {
		return PATH_TYPE_FILESTORE_DB_JSON, name[:len(name)-8]
	}

	if strings.HasSuffix(name, ".idx") {
		return PATH_TYPE_FILESTORE_SPARSE_IDX, name[:len(name)-4]
	}

	if strings.HasSuffix(name, ".zip") {
		return PATH_TYPE_FILESTORE_DOWNLOAD_ZIP, name[:len(name)-4]
	}

	if strings.HasSuffix(name, ".chunk") {
		return PATH_TYPE_FILESTORE_CHUNK_INDEX, name[:len(name)-7]
	}

	if strings.HasSuffix(name, ".html") {
		return PATH_TYPE_FILESTORE_DOWNLOAD_REPORT, name[:len(name)-5]
	}

	if strings.HasSuffix(name, ".tmp") {
		return PATH_TYPE_FILESTORE_TMP, name[:len(name)-4]
	}

	if strings.HasSuffix(name, ".csv") {
		return PATH_TYPE_FILESTORE_CSV, name[:len(name)-4]
	}

	if strings.HasSuffix(name, ".db") {
		return PATH_TYPE_FILESTORE_DB, name[:len(name)-3]
	}

	if strings.HasSuffix(name, ".yaml") {
		return PATH_TYPE_FILESTORE_YAML, name[:len(name)-5]
	}

	return PATH_TYPE_FILESTORE_ANY, name
}
