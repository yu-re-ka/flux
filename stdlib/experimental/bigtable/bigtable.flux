package bigtable


// bigtable.from is a function that retrieves data from a Google Cloud
// Bigtable data source.
//
// - `token` is the Google Cloud IAM token used to access the Cloud
//	Bigtable database.
// - `project` is the project ID of the Cloud Bigtable project to
//	retrieve data from.
// - `instance` is the instance ID of the Cloud Bigtable instance to
//	retrieve data from.
// - `table` is the bame of the Cloud Bigtable table to retrieve data from.
//
builtin from : (token: string, project: string, instance: string, table: string) => [T] where T: Record
