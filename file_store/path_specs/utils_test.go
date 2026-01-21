package path_specs

import (
	"testing"

	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type testCaseT struct {
	components  []string
	client_path string
	path_type   api.PathType
}

func TestFromGenericComponentList(t *testing.T) {
	for _, tc := range []testCaseT{
		{
			components: []string{"downloads",
				"server", "F.D1CDBN7O9PTU4",
				"server-server-F.D1CDBN7O9PTU4.zip"},
			client_path: "/downloads/server/F.D1CDBN7O9PTU4/server-server-F.D1CDBN7O9PTU4.zip",
			path_type:   api.PATH_TYPE_FILESTORE_DOWNLOAD_ZIP,
		},
		{
			// Public directory is always PATH_TYPE_FILESTORE_ANY
			components:  []string{"public", "test.zip"},
			client_path: "/public/test.zip",
			path_type:   api.PATH_TYPE_FILESTORE_ANY,
		},

		{
			// Global notebook attachments are always PATH_TYPE_FILESTORE_ANY
			components: []string{
				"notebooks", "N.D55OJV2COB544", "attach", "NA.D56I70FP35OKU-file.zip"},
			client_path: "/notebooks/N.D55OJV2COB544/attach/NA.D56I70FP35OKU-file.zip",
			path_type:   api.PATH_TYPE_FILESTORE_ANY,
		},
		{
			// Global notebook uploads are always PATH_TYPE_FILESTORE_ANY
			components: []string{
				"notebooks", "N.D55OJV2COB544", "NC.D56I6S6LK00FI-D56I71V0BJULE",
				"uploads", "data", "file.zip"},
			client_path: "/notebooks/N.D55OJV2COB544/NC.D56I6S6LK00FI-D56I71V0BJULE/uploads/data/file.zip",
			path_type:   api.PATH_TYPE_FILESTORE_ANY,
		},
		{
			// Client notebook attachments are always PATH_TYPE_FILESTORE_ANY
			components: []string{
				"clients", "C.d7f8859f5e0e01f7", "collections", "F.D55T34A0NIDTC",
				"notebook", "N.F.D55T34A0NIDTC-C.d7f8859f5e0e01f7", "attach",
				"NA.D56HQ0GRL4CK0-file.zip"},
			client_path: "/clients/C.d7f8859f5e0e01f7/collections/F.D55T34A0NIDTC/notebook/N.F.D55T34A0NIDTC-C.d7f8859f5e0e01f7/attach/NA.D56HQ0GRL4CK0-file.zip",
			path_type:   api.PATH_TYPE_FILESTORE_ANY,
		},
		{
			// Client notebook uploads are always PATH_TYPE_FILESTORE_ANY
			components: []string{
				"clients", "C.d7f8859f5e0e01f7", "collections", "F.D55T34A0NIDTC",
				"notebook", "N.F.D55T34A0NIDTC-C.d7f8859f5e0e01f7",
				"NC.D56CPJR8RE9GO-D56HRHRS9QR6A", "uploads", "file",
				"file.zip"},
			client_path: "/clients/C.d7f8859f5e0e01f7/collections/F.D55T34A0NIDTC/notebook/N.F.D55T34A0NIDTC-C.d7f8859f5e0e01f7/NC.D56CPJR8RE9GO-D56HRHRS9QR6A/uploads/file/file.zip",
			path_type:   api.PATH_TYPE_FILESTORE_ANY,
		},

		{
			// Hunt notebook attachments are always PATH_TYPE_FILESTORE_ANY
			components: []string{
				"hunts", "H.D4ASRT5R531G4", "notebook", "N.H.D4ASRT5R531G4",
				"attach", "NA.D56HQ0GRL4CK0-file.zip"},
			client_path: "/hunts/H.D4ASRT5R531G4/notebook/N.H.D4ASRT5R531G4/attach/NA.D56HQ0GRL4CK0-file.zip",
			path_type:   api.PATH_TYPE_FILESTORE_ANY,
		},
		{
			// Client notebook uploads are always PATH_TYPE_FILESTORE_ANY
			components: []string{
				"hunts", "H.D4ASRT5R531G4", "notebook", "N.H.D4ASRT5R531G4",
				"NC.D4ASRVHMIJFK0-D56I6463RE2QU", "uploads", "data",
				"file.zip"},
			client_path: "/hunts/H.D4ASRT5R531G4/notebook/N.H.D4ASRT5R531G4/NC.D4ASRVHMIJFK0-D56I6463RE2QU/uploads/data/file.zip",
			path_type:   api.PATH_TYPE_FILESTORE_ANY,
		},
	} {
		path := FromGenericComponentList(tc.components)
		assert.Equal(t, path.Type(), tc.path_type,
			"PathType is not correct: %v vs %v (%v)",
			path.Type(), tc.path_type, tc.components)

		assert.Equal(t, path.AsClientPath(), tc.client_path,
			"ClietPath is not correct: %v vs %v",
			path.AsClientPath(), tc.client_path)
	}
}

// Test that FromGenericComponentList does not mutate the passed components slice.
func TestFromGenericComponentListImmutable(t *testing.T) {
	components := []string{"downloads", "C.3c4f2d6cfc5d7219", "F.D5NVNO28H7U54", "RAPTOR-C.3c4f2d6cfc5d7219-F.D5NVNO28H7U54.zip"}
	expected := []string{"downloads", "C.3c4f2d6cfc5d7219", "F.D5NVNO28H7U54", "RAPTOR-C.3c4f2d6cfc5d7219-F.D5NVNO28H7U54.zip"}

	_ = FromGenericComponentList(components)
	assert.Equal(t, components, expected, "slice was mutated: got %v; expected %v", components, expected)
}
