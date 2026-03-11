package s3

// This is an S3 accessor

// Sample query:

// LET S3_CREDENTIALS <= dict(endpoint='http://127.0.0.1:4566/', credentials_key='admin', credentials_secret='password', no_verify_cert=1)
// SELECT *, read_file(filename=OSPath, length=10, accessor='s3') AS Data  FROM glob(globs='/velociraptor/orgs/root/clients/C.39a107c4c58c5efa/collections/*/uploads/auto/*', accessor='s3')

// This accessor has two versions:

// 1. The one built with the official aws client library is full
//    featured but comes with an increased binary size. Build this
//    version using the sumo option.
// 2. The default one is built using the leaner minio library.
