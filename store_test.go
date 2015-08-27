package libnetwork

import (
	"net"
	"os"
	"testing"

	"github.com/docker/libkv/store"
	"github.com/docker/libnetwork/config"
	"github.com/docker/libnetwork/datastore"
	"github.com/docker/libnetwork/netlabel"
	"github.com/docker/libnetwork/options"
)

func TestBoltdbBackend(t *testing.T) {
	defer os.Remove("/tmp/boltdb.db")
	testLocalBackend(t, "", "", nil)
	os.Remove(defaultLocalStoreConfig.Client.Address)
	testLocalBackend(t, "boltdb", "/tmp/boltdb.db", &store.Config{Bucket: "testBackend"})
}

func testLocalBackend(t *testing.T, provider, url string, storeConfig *store.Config) {
	netOptions := []config.Option{}
	netOptions = append(netOptions, config.OptionLocalKVProvider(provider))
	netOptions = append(netOptions, config.OptionLocalKVProviderURL(url))
	netOptions = append(netOptions, config.OptionLocalKVProviderConfig(storeConfig))

	ctrl, err := New(netOptions...)
	if err != nil {
		t.Fatalf("Error new controller: %v", err)
	}
	err = ctrl.Start()
	if err != nil {
		t.Fatalf("Error starting controller: %v", err)
	}
	if err := ctrl.ConfigureNetworkDriver("host", options.NewGeneric()); err != nil {
		t.Fatalf("Error initializing host driver: %v", err)
	}
	nw, err := ctrl.NewNetwork("host", "host")
	if err != nil {
		t.Fatalf("Error creating default \"host\" network: %v", err)
	}
	ep, err := nw.CreateEndpoint("newendpoint", []EndpointOption{}...)
	if err != nil {
		t.Fatalf("Error creating endpoint: %v", err)
	}
	store := ctrl.(*controller).localStore.KVStore()
	if exists, err := store.Exists(datastore.Key(datastore.NetworkKeyPrefix, string(nw.ID()))); !exists || err != nil {
		t.Fatalf("Network key should have been created.")
	}
	if exists, err := store.Exists(datastore.Key([]string{datastore.EndpointKeyPrefix, string(nw.ID()), string(ep.ID())}...)); !exists || err != nil {
		t.Fatalf("Endpoint key should have been created.")
	}
	store.Close()

	// test restore of local store
	ctrl, err = New(netOptions...)
	if err != nil {
		t.Fatalf("Error creating controller: %v", err)
	}
	err = ctrl.Start()
	if err != nil {
		t.Fatalf("Error starting controller: %v", err)
	}
	if nw, err = ctrl.NetworkByID(nw.ID()); err != nil {
		t.Fatalf("Error get network %v", err)
	}
	if ep, err = nw.EndpointByID(ep.ID()); err != nil {
		t.Fatalf("Error get endpoint %v", err)
	}
	if err := ep.Delete(); err != nil {
		t.Fatalf("Error delete endpoint %v", err)
	}
	store = ctrl.(*controller).localStore.KVStore()
	if exists, err := store.Exists(datastore.Key([]string{datastore.EndpointKeyPrefix, string(nw.ID()), string(ep.ID())}...)); exists || err != nil {
		t.Fatalf("Endpoint key should have been deleted. ")
	}
}

func TestLocalRestore(t *testing.T) {
	defer os.Remove(defaultLocalStoreConfig.Client.Address)
	netOptions := []config.Option{}
	netOptions = append(netOptions, config.OptionLocalKVProvider(""))

	ctrl, err := New(netOptions...)
	if err != nil {
		t.Fatalf("Error new controller: %v", err)
	}

	if err := ctrl.ConfigureNetworkDriver("bridge", options.Generic{netlabel.GenericData: options.Generic{}}); err != nil {
		t.Fatalf("Error initializing bridge driver: %v", err)
	}
	_, bipNet, err := net.ParseCIDR("172.18.42.1/16")
	if err != nil {
		t.Fatalf("Erorr %v", err)
	}
	netOption := options.Generic{
		"BridgeName":         "docker0",
		"EnableIPMasquerade": true,
		"EnableICC":          true,
		"AddressIPv4":        bipNet,
	}
	// Initialize default network on "bridge" with the same name
	nw, err := ctrl.NewNetwork("bridge", "bridge", NetworkOptionGeneric(options.Generic{netlabel.GenericData: netOption}))
	if err != nil {
		t.Fatalf("Error creating default \"bridge\" network: %v", err)
	}
	store := ctrl.(*controller).localStore.KVStore()
	if exists, err := store.Exists(datastore.Key(datastore.NetworkKeyPrefix, string(nw.ID()))); !exists || err != nil {
		t.Fatalf("Network key should have been created.")
	}
	err = ctrl.Start()
	if err != nil {
		t.Fatalf("Error starting controller: %v", err)
	}

	ep, err := nw.CreateEndpoint("newendpoint", []EndpointOption{}...)
	if err != nil {
		t.Fatalf("Error creating endpoint: %v", err)
	}
	if exists, err := store.Exists(datastore.Key([]string{datastore.EndpointKeyPrefix, string(nw.ID()), string(ep.ID())}...)); !exists || err != nil {
		t.Fatalf("Endpoint key should have been created. ")
	}
	store.Close()

	ctrl, err = New(netOptions...)
	if err != nil {
		t.Fatalf("Error new controller: %v", err)
	}

	if err := ctrl.ConfigureNetworkDriver("bridge", options.Generic{netlabel.GenericData: options.Generic{}}); err != nil {
		t.Fatalf("Error initializing bridge driver: %v", err)
	}
	nw, err = ctrl.NewNetwork("bridge", "bridge", NetworkOptionGeneric(options.Generic{netlabel.GenericData: netOption}))
	if err != nil {
		t.Fatalf("Error creating default \"bridge\" network: %v", err)
	}

	err = ctrl.Start()
	if err != nil {
		t.Fatalf("Error starting controller: %v", err)
	}

	if len(nw.Endpoints()) != 1 {
		t.Fatalf("num of endpoints should be 1")
	}
}
