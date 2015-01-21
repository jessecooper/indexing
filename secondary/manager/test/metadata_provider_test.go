// Copyright (c) 2014 Couchbase, Inc.
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
// except in compliance with the License. You may obtain a copy of the License at
//   http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed under the
// License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
// either express or implied. See the License for the specific language governing permissions
// and limitations under the License.

package test 

import (
	"fmt"
	"github.com/couchbase/indexing/secondary/common"
	"github.com/couchbase/indexing/secondary/manager/client"
	"github.com/couchbase/indexing/secondary/manager"
	util "github.com/couchbase/indexing/secondary/manager/test/util"
	"testing"
	"time"
)

// For this test, use Index Defn Id from 100 - 110
func TestMetadataProvider(t *testing.T) {

	common.LogEnable()
	common.SetLogLevel(common.LogLevelDebug)

	common.Infof("Start Index Manager *********************************************************")

	var msgAddr = "localhost:9884"
	factory := new(util.TestDefaultClientFactory)
	env := new(util.TestDefaultClientEnv)
	admin := manager.NewProjectorAdmin(factory, env, nil)
	mgr, err := manager.NewIndexManagerInternal(msgAddr, "localhost:" + manager.COORD_MAINT_STREAM_PORT, admin)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	common.Infof("Cleanup Test *********************************************************")

	cleanupTest(mgr, t)

	common.Infof("Setup Initial Data *********************************************************")

	setupInitialData(mgr, t)

	common.Infof("Start Provider *********************************************************")

	var providerId = "TestMetadataProvider"
	provider, err := client.NewMetadataProvider(providerId)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	provider.WatchMetadata(msgAddr)

	// the gometa server is running in the same process as MetadataProvider (client).  So sleep to 
	// make sure that the server has a chance to finish off initialization, since the client may 
	// be ready, but the server is not.
	time.Sleep(time.Duration(1000) * time.Millisecond)

	common.Infof("Verify Initial Data *********************************************************")

	meta := lookup(provider, common.IndexDefnId(100)) 
	if meta == nil {
		t.Fatal("Cannot find Index Defn 100 from MetadataProvider")
	}
	common.Infof("found Index Defn 100")
	if len(meta.Instances) == 0 || meta.Instances[0].State != common.INDEX_STATE_READY {
		t.Fatal("Index Defn 100 state is not ready")
	}

	meta = lookup(provider, common.IndexDefnId(101)) 
	if meta == nil {
		t.Fatal("Cannot find Index Defn 101 from MetadataProvider")
	}
	common.Infof("found Index Defn 101")
	if len(meta.Instances) == 0 || meta.Instances[0].State != common.INDEX_STATE_READY {
		t.Fatal("Index Defn 101 state is not ready")
	}

	common.Infof("Change Data *********************************************************")

	newDefnId, err := provider.CreateIndex("metadata_provider_test_102", "Default", common.ForestDB,
		common.N1QL, "Testing", "Testing", msgAddr, []string{"Testing"}, false)
	if err != nil {
		t.Fatal("Cannot create Index Defn 102 through MetadataProvider")
	}

	if err := provider.DropIndex(common.IndexDefnId(101), msgAddr); err != nil {
		t.Fatal("Cannot drop Index Defn 101 through MetadataProvider")
	}

	common.Infof("Verify Changed Data *********************************************************")

	if lookup(provider, common.IndexDefnId(100)) == nil {
		t.Fatal("Cannot find Index Defn 100 from MetadataProvider")
	}
	common.Infof("found Index Defn 100")

	if lookup(provider, common.IndexDefnId(101)) != nil {
		t.Fatal("Found Deleted Index Defn 101 from MetadataProvider")
	}
	common.Infof("cannot found deleted Index Defn 101")

	if lookup(provider, newDefnId) == nil {
		t.Fatal(fmt.Sprintf("Cannot Found Index Defn %d from MetadataProvider", newDefnId))
	}
	common.Infof("Found Index Defn %d", newDefnId)

	common.Infof("Cleanup Test *********************************************************")

	cleanupTest(mgr, t)
	cleanSingleIndex(mgr, t, newDefnId) 
	time.Sleep(time.Duration(1000) * time.Millisecond)
}

func lookup(provider *client.MetadataProvider, id common.IndexDefnId) *client.IndexMetadata {

	metas := provider.ListIndex()

	for _, meta := range metas {
		if meta.Definition.DefnId == id {
			return meta
		}
	}

	return nil
}

func setupInitialData(mgr *manager.IndexManager, t *testing.T) {

	// Add a new index definition : 100
	idxDefn := &common.IndexDefn{
		DefnId:          common.IndexDefnId(100),
		Name:            "metadata_provider_test_100",
		Using:           common.ForestDB,
		Bucket:          "Default",
		IsPrimary:       false,
		SecExprs:        []string{"Testing"},
		ExprType:        common.N1QL,
		PartitionScheme: common.HASH,
		PartitionKey:    "Testing"}

	err := mgr.HandleCreateIndexDDL(idxDefn)
	if err != nil {
		t.Fatal(err)
	}

	// Add a new index definition : 101
	idxDefn = &common.IndexDefn{
		DefnId:          common.IndexDefnId(101),
		Name:            "metadata_provider_test_101",
		Using:           common.ForestDB,
		Bucket:          "Default",
		IsPrimary:       false,
		SecExprs:        []string{"Testing"},
		ExprType:        common.N1QL,
		PartitionScheme: common.HASH,
		PartitionKey:    "Testing"}

	err = mgr.HandleCreateIndexDDL(idxDefn)
	if err != nil {
		t.Fatal(err)
	}
	
	// Update the index definition to ready
	topology, err := mgr.GetTopologyByBucket("Default")
	if err != nil {
		util.TT.Fatal(err)
	}
	
	topology.ChangeStateForIndexInstByDefn(common.IndexDefnId(100), common.INDEX_STATE_CREATED, common.INDEX_STATE_READY)
	topology.ChangeStateForIndexInstByDefn(common.IndexDefnId(101), common.INDEX_STATE_CREATED, common.INDEX_STATE_READY)
	if err := mgr.SetTopologyByBucket("Default", topology); err != nil {
		util.TT.Fatal(err)
	}
}

// clean up
func cleanupTest(mgr *manager.IndexManager, t *testing.T) {

	cleanSingleIndex(mgr, t, common.IndexDefnId(100))
	cleanSingleIndex(mgr, t, common.IndexDefnId(101))
}

func cleanSingleIndex(mgr *manager.IndexManager, t *testing.T, id common.IndexDefnId) {

	_, err := mgr.GetIndexDefnById(id)
	if err != nil {
		common.Infof("cleanupTest() :  cannot find index defn %d.  No cleanup ...", id)
	} else {
		common.Infof("cleanupTest.cleanupTest() :  found index defn %d.  Cleaning up ...", id)

		mgr.HandleDeleteIndexDDL(id)

		_, err := mgr.GetIndexDefnById(id)
		if err == nil {
			common.Infof("cleanupTest() :  cannot cleanup index defn %d.  ...", id)
		}
	}
}