/*
 (c) Copyright [2021] Micro Focus or one of its affiliates.
 Licensed under the Apache License, Version 2.0 (the "License");
 You may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package vdbgen

import (
	"context"
	"database/sql"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vapi "github.com/vertica/vertica-kubernetes/api/v1beta1"
	"github.com/vertica/vertica-kubernetes/pkg/controllers"
)

var (
	db   *sql.DB
	mock sqlmock.Sqlmock
)

func createMock() {
	var err error
	db, mock, err = sqlmock.New(sqlmock.MonitorPingsOption(true))
	Expect(err).Should(Succeed())
}

func deleteMock() {
	db.Close()
}

var _ = Describe("vdb", func() {
	ctx := context.Background()

	It("should init vdb from options", func() {
		createMock()
		defer deleteMock()

		dbGen := DBGenerator{Conn: db, Opts: &Options{
			DBName:             "mydb",
			VdbName:            "vertdb",
			Image:              "my-img:latest",
			IgnoreClusterLease: true,
		}}
		dbGen.setParmsFromOptions()
		Expect(string(dbGen.Objs.Vdb.Spec.InitPolicy)).Should(Equal(vapi.CommunalInitPolicyRevive))
		Expect(dbGen.Objs.Vdb.Spec.DBName).Should(Equal("mydb"))
		Expect(dbGen.Objs.Vdb.ObjectMeta.Name).Should(Equal("vertdb"))
		Expect(dbGen.Objs.Vdb.Spec.IgnoreClusterLease).Should(BeTrue())
		Expect(dbGen.Objs.Vdb.Spec.Image).Should(Equal("my-img:latest"))
	})

	It("should call ping() when we connect", func() {
		createMock()
		defer deleteMock()

		dbGen := DBGenerator{Conn: db, Opts: &Options{}}

		mock.ExpectPing()
		Expect(dbGen.connect(ctx)).Should(Succeed())
		Expect(mock.ExpectationsWereMet()).Should(Succeed())
	})

	It("should set shard count from sql query", func() {
		createMock()
		defer deleteMock()

		dbGen := DBGenerator{Conn: db}

		mock.ExpectQuery("SELECT COUNT.* FROM SHARDS .*").
			WillReturnRows(sqlmock.NewRows([]string{"1"}).FromCSVString("12"))

		Expect(dbGen.setShardCount(ctx)).Should(Succeed())
		Expect(dbGen.Objs.Vdb.Spec.ShardCount).Should(Equal(12))
		Expect(mock.ExpectationsWereMet()).Should(Succeed())
	})

	It("should get communal endpoint from show database", func() {
		createMock()
		defer deleteMock()

		dbGen := DBGenerator{Conn: db}

		mock.ExpectQuery(Queries[DBCfgKey]).
			WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
				AddRow("AWSEndpoint", "minio:30312").
				AddRow("AWSEnableHttps", "0").
				AddRow("other", "value").
				AddRow("AWSAuth", "minio:minio123"))
		Expect(dbGen.fetchDatabaseConfig(ctx)).Should(Succeed())
		Expect(dbGen.setCommunalEndpoint(ctx)).Should(Succeed())
		Expect(dbGen.Objs.Vdb.Spec.Communal.Endpoint).Should(Equal("http://minio:30312"))
		Expect(dbGen.Objs.CredSecret.Data[controllers.S3AccessKeyName]).Should(Equal([]byte("minio")))
		Expect(dbGen.Objs.CredSecret.Data[controllers.S3SecretKeyName]).Should(Equal([]byte("minio123")))

		mock.ExpectQuery(Queries[DBCfgKey]).
			WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
				AddRow("AWSEndpoint", "192.168.0.1").
				AddRow("AWSEnableHttps", "1").
				AddRow("other", "value").
				AddRow("AWSAuth", "auth:secret"))
		Expect(dbGen.fetchDatabaseConfig(ctx)).Should(Succeed())
		Expect(dbGen.setCommunalEndpoint(ctx)).Should(Succeed())
		Expect(dbGen.Objs.Vdb.Spec.Communal.Endpoint).Should(Equal("https://192.168.0.1"))

		Expect(mock.ExpectationsWereMet()).Should(Succeed())
	})

	It("should extract common prefix for local and depot path", func() {
		createMock()
		defer deleteMock()

		dbGen := DBGenerator{Conn: db}

		mock.ExpectQuery(Queries[StorageLocationKey]).
			WithArgs("DATA,TEMP").
			WillReturnRows(sqlmock.NewRows([]string{"node_name", "location_path"}).
				AddRow("v_vertdb_node0001", "/data/vertdb/v_vertdb_node0001_data").
				AddRow("v_vertdb_node0002", "/data/vertdb/v_vertdb_node0002_data"))
		mock.ExpectQuery(Queries[StorageLocationKey]).
			WithArgs("DEPOT").
			WillReturnRows(sqlmock.NewRows([]string{"node_name", "location_path"}).
				AddRow("v_vertdb_node0001", "/home/dbadmin/depot/vertdb/v_vertdb_node0001_data").
				AddRow("v_vertdb_node0002", "/home/dbadmin/depot/vertdb/v_vertdb_node0002_data"))

		Expect(dbGen.setLocalPaths(ctx)).Should(Succeed())
		Expect(dbGen.Objs.Vdb.Spec.Local.DataPath).Should(Equal("/data"))
		Expect(dbGen.Objs.Vdb.Spec.Local.DepotPath).Should(Equal("/home/dbadmin/depot"))

		Expect(mock.ExpectationsWereMet()).Should(Succeed())
	})

	It("should raise an error if the local paths are different on two nodes", func() {
		createMock()
		defer deleteMock()

		dbGen := DBGenerator{Conn: db}

		mock.ExpectQuery(Queries[StorageLocationKey]).
			WithArgs("DATA,TEMP").
			WillReturnRows(sqlmock.NewRows([]string{"node_name", "location_path"}).
				AddRow("v_vertdb_node0001", "/data1/vertdb/v_vertdb_node0001_data").
				AddRow("v_vertdb_node0002", "/data2/vertdb/v_vertdb_node0002_data"))

		Expect(dbGen.setLocalPaths(ctx)).ShouldNot(Succeed())
		Expect(mock.ExpectationsWereMet()).Should(Succeed())
	})

	It("should find subcluster detail", func() {
		createMock()
		defer deleteMock()

		dbGen := DBGenerator{Conn: db}

		const Sc1Name = "sc1"
		const Sc2Name = "sc2"
		const Sc3Name = "sc3"

		mock.ExpectQuery(Queries[SubclusterQueryKey]).
			WillReturnRows(sqlmock.NewRows([]string{"node_name", "location_path"}).
				AddRow(Sc1Name, true).AddRow(Sc1Name, true).AddRow(Sc1Name, true).
				AddRow(Sc2Name, false).AddRow(Sc1Name, true).AddRow(Sc3Name, true).
				AddRow(Sc3Name, true))

		expScDetail := []vapi.Subcluster{
			{Name: Sc1Name, Size: 4, IsPrimary: true},
			{Name: Sc2Name, Size: 1, IsPrimary: false},
			{Name: Sc3Name, Size: 2, IsPrimary: true},
		}
		expReviveOrder := []vapi.SubclusterPodCount{
			{SubclusterIndex: 0, PodCount: 3},
			{SubclusterIndex: 1, PodCount: 1},
			{SubclusterIndex: 0, PodCount: 1},
			{SubclusterIndex: 2, PodCount: 2},
		}

		Expect(dbGen.setSubclusterDetail(ctx)).Should(Succeed())
		Expect(dbGen.Objs.Vdb.Spec.Subclusters).Should(Equal(expScDetail))
		Expect(dbGen.Objs.Vdb.Spec.ReviveOrder).Should(Equal(expReviveOrder))
		Expect(mock.ExpectationsWereMet()).Should(Succeed())
	})

	It("should fail if subcluster name is not suitable for Kubernetes", func() {
		createMock()
		defer deleteMock()

		dbGen := DBGenerator{Conn: db}

		for _, name := range []string{"default_subcluster", "scCapital", "sc!bang", "sc-"} {
			mock.ExpectQuery(Queries[SubclusterQueryKey]).
				WillReturnRows(sqlmock.NewRows([]string{"node_name", "location_path"}).AddRow(name, true))

			Expect(dbGen.setSubclusterDetail(ctx)).ShouldNot(Succeed(), name)
			Expect(mock.ExpectationsWereMet()).Should(Succeed())
		}
	})

	It("should find communal path from storage location", func() {
		createMock()
		defer deleteMock()

		dbGen := DBGenerator{Conn: db}

		const expCommunalPath = "s3://nimbusdb/db/mspilchen"
		mock.ExpectQuery(Queries[StorageLocationKey]).
			WithArgs("DATA").
			WillReturnRows(sqlmock.NewRows([]string{"node_name", "location_path"}).
				AddRow("", expCommunalPath))

		Expect(dbGen.setCommunalPath(ctx)).Should(Succeed())
		Expect(dbGen.Objs.Vdb.Spec.Communal.Path).Should(Equal(expCommunalPath))
		Expect(mock.ExpectationsWereMet()).Should(Succeed())
	})

	It("should include license if license data is present", func() {
		dbGen := DBGenerator{LicenseData: []byte("test"), Opts: &Options{}}
		dbGen.setParmsFromOptions()
		Expect(dbGen.setLicense(ctx)).Should(Succeed())
		Expect(len(dbGen.Objs.LicenseSecret.Data)).ShouldNot(Equal(0))
		Expect(len(dbGen.Objs.Vdb.Spec.LicenseSecret)).ShouldNot(Equal(0))
		Expect(dbGen.Objs.Vdb.Spec.LicenseSecret).Should(Equal(dbGen.Objs.LicenseSecret.ObjectMeta.Name))
	})

	It("should fail if CA file isn't present but one is in the db cfg", func() {
		createMock()
		defer deleteMock()

		dbGen := DBGenerator{Conn: db, Opts: &Options{}}

		mock.ExpectQuery(Queries[DBCfgKey]).
			WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
				AddRow("AWSEndpoint", "minio:30312").
				AddRow("AWSEnableHttps", "1").
				AddRow("AWSCAFile", "/certs/ca.crt").
				AddRow("AWSAuth", "minio:minio123"))

		Expect(dbGen.fetchDatabaseConfig(ctx)).Should(Succeed())
		Expect(dbGen.setCAFile(ctx)).ShouldNot(Succeed())

		// Now correct the error by providing a ca file in the opts.
		dbGen.Opts.CAFile = "ca.crt"
		Expect(dbGen.setCAFile(ctx)).Should(Succeed())
		Expect(dbGen.Objs.HasCAFile).Should(BeTrue())
		Expect(len(dbGen.Objs.Vdb.Spec.CertSecrets)).Should(Equal(1))
	})
})
