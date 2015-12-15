package bridge

import (
	"encoding/json"
	"net"

	"github.com/Sirupsen/logrus"
	"github.com/docker/libnetwork/datastore"
	"github.com/docker/libnetwork/types"
)

const bridgeEndpointPrefix = "bridge_endpoint"

func (n *bridgeNetwork) populateEndpoints() error {
	eps, err := n.getEndpointsFromStore()
	if err != nil {
		return err
	}
	for _, ep := range eps {
		ep.network = n
		//Do not need to restore ports here, cause port allocator won't allocate an used port
		n.Lock()
		n.endpoints[ep.id] = ep
		n.Unlock()
	}
	return nil
}

func (n *bridgeNetwork) getEndpointsFromStore() ([]*bridgeEndpoint, error) {
	var epl []*bridgeEndpoint

	tmp := bridgeEndpoint{network: n}
	kvol, err := n.driver.store.List(datastore.Key(tmp.KeyPrefix()...), &bridgeEndpoint{network: n})
	// Continue searching in the next store if no keys found in this store
	if err != nil {
		if err != datastore.ErrKeyNotFound {
			logrus.Debugf("failed to get bridge endpoints for network %s: %v", n.id, err)
			return epl, err
		}
	}

	for _, kvo := range kvol {
		ep := kvo.(*bridgeEndpoint)
		epl = append(epl, ep)
	}

	return epl, nil
}

func (ep *bridgeEndpoint) MarshalJSON() ([]byte, error) {
	var pms []string
	nMap := make(map[string]interface{})
	nMap["id"] = ep.id
	nMap["srcName"] = ep.srcName
	if ep.addr != nil {
		nMap["addr"] = ep.addr.String()
	}
	if ep.addrv6 != nil {
		nMap["addrv6"] = ep.addrv6.String()
	}
	if len(ep.macAddress) != 0 {
		nMap["macAddress"] = ep.macAddress.String()
	}
	nMap["config"] = ep.config
	nMap["containerConfiguration"] = ep.containerConfig
	if len(ep.portMapping) != 0 {
		for _, pm := range ep.portMapping {
			pms = append(pms, pm.String())
		}
		nMap["portMapping"] = pms
	}
	return json.Marshal(nMap)
}

func (ep *bridgeEndpoint) UnmarshalJSON(b []byte) error {
	var (
		err  error
		nMap map[string]interface{}
		cfg  *endpointConfiguration
		ccfg *containerConfiguration
		pms  []types.PortBinding
	)
	if err = json.Unmarshal(b, &nMap); err != nil {
		return err
	}
	ep.id = nMap["id"].(string)
	ep.srcName = nMap["srcName"].(string)
	if _, ok := nMap["addr"]; ok {
		if ep.addr, err = types.ParseCIDR(nMap["addr"].(string)); err != nil {
			return types.InternalErrorf("failed to decode bridge endpoint address IPv4 after json unmarshal: %s", nMap["addr"].(string))
		}
	}
	if _, ok := nMap["addrv6"]; ok {
		if ep.addrv6, err = types.ParseCIDR(nMap["addrv6"].(string)); err != nil {
			return types.InternalErrorf("failed to decode bridge endpoint address IPv6 after json unmarshal: %s", nMap["addrv6"].(string))
		}
	}
	if _, ok := nMap["macAddress"]; ok {
		if ep.macAddress, err = net.ParseMAC(nMap["macAddress"].(string)); err != nil {
			return types.InternalErrorf("failed to decode bridge endpoint mac address after json unmarshal: %s", nMap["macAddress"].(string))
		}
	}
	configData, err := json.Marshal(nMap["config"])
	if err != nil {
		return types.InternalErrorf("failed to decode bridge endpoint config after json unmarshal %v: %v", nMap["config"], err)
	}
	if err = json.Unmarshal(configData, &cfg); err != nil {
		return types.InternalErrorf("failed to decode bridge endpoint config after json unmarshal %v: %v", nMap["config"], err)
	}
	ep.config = cfg
	containerConfigData, err := json.Marshal(nMap["containerConfiguration"])
	if err != nil {
		return types.InternalErrorf("failed to decode bridge endpoint container configuration after json unmarshal %v: %v", nMap["containerConfiguration"], err)
	}
	if err = json.Unmarshal(containerConfigData, &ccfg); err != nil {
		return types.InternalErrorf("failed to decode bridge endpoint container configuration after json unmarshal %v: %v", nMap["containerConfiguration"], err)
	}
	ep.containerConfig = ccfg
	if _, ok := nMap["portMapping"]; ok {
		for _, str := range nMap["portMapping"].([]string) {
			pm := &types.PortBinding{}
			if err = pm.FromString(str); err != nil {
				return types.InternalErrorf("failed to decode bridge endpoint port mapping after json unmarshal: %s", str)
			}
			pms = append(pms, *pm)
		}
	}
	ep.portMapping = pms
	return nil
}

func (ep *bridgeEndpoint) Key() []string {
	return []string{bridgeEndpointPrefix, ep.network.id, ep.id}
}

func (ep *bridgeEndpoint) KeyPrefix() []string {
	return []string{bridgeEndpointPrefix, ep.network.id}
}

func (ep *bridgeEndpoint) Value() []byte {
	b, err := json.Marshal(ep)
	if err != nil {
		return nil
	}
	return b
}

func (ep *bridgeEndpoint) SetValue(value []byte) error {
	return json.Unmarshal(value, ep)
}

func (ep *bridgeEndpoint) Index() uint64 {
	return ep.dbIndex
}

func (ep *bridgeEndpoint) SetIndex(index uint64) {
	ep.dbIndex = index
	ep.dbExists = true
}

func (ep *bridgeEndpoint) Exists() bool {
	return ep.dbExists
}

func (ep *bridgeEndpoint) Skip() bool {
	return false
}

func (ep *bridgeEndpoint) New() datastore.KVObject {
	return &bridgeEndpoint{network: ep.network}
}

func (ep *bridgeEndpoint) CopyTo(o datastore.KVObject) error {
	dstEp := o.(*bridgeEndpoint)
	dstEp.id = ep.id
	dstEp.srcName = ep.srcName
	dstEp.addr = types.GetIPNetCopy(ep.addr)
	dstEp.addrv6 = types.GetIPNetCopy(ep.addrv6)
	dstEp.macAddress = types.GetMacCopy(ep.macAddress)
	if ep.config != nil {
		dstEp.config = &endpointConfiguration{}
		ep.config.CopyTo(dstEp.config)
	}
	if ep.containerConfig != nil {
		dstEp.containerConfig = &containerConfiguration{}
		ep.containerConfig.CopyTo(dstEp.containerConfig)
	}
	dstEp.portMapping = make([]types.PortBinding, len(ep.portMapping))
	copy(dstEp.portMapping, ep.portMapping)
	return nil
}

func (ep *bridgeEndpoint) DataScope() string {
	return datastore.LocalScope
}

func (cf *containerConfiguration) MarshalJSON() ([]byte, error) {
	cMap := make(map[string]interface{})
	cMap["ParentEndpoints"] = cf.ParentEndpoints
	cMap["ChildEndpoints"] = cf.ChildEndpoints
	return json.Marshal(cMap)
}

func (cf *containerConfiguration) UnmarshalJSON(b []byte) error {
	var (
		err  error
		cMap map[string]interface{}
	)
	if err = json.Unmarshal(b, &cMap); err != nil {
		return err
	}
	cf.ParentEndpoints = cMap["ParentEndpoints"].([]string)
	cf.ChildEndpoints = cMap["ChildEndpoints"].([]string)
	return nil
}

func (cc *containerConfiguration) CopyTo(dstCc *containerConfiguration) error {
	dstCc.ParentEndpoints = make([]string, len(cc.ParentEndpoints))
	copy(dstCc.ParentEndpoints, cc.ParentEndpoints)
	dstCc.ChildEndpoints = make([]string, len(cc.ChildEndpoints))
	copy(dstCc.ChildEndpoints, cc.ChildEndpoints)
	return nil
}

func (ec *endpointConfiguration) MarshalJSON() ([]byte, error) {
	var pms, eps []string
	cMap := make(map[string]interface{})
	if len(ec.MacAddress) != 0 {
		cMap["MacAddress"] = ec.MacAddress.String()
	}
	if len(ec.PortBindings) != 0 {
		for _, pm := range ec.PortBindings {
			pms = append(pms, pm.String())
		}
		cMap["PortBindings"] = pms
	}
	if len(ec.ExposedPorts) != 0 {
		for _, ep := range ec.ExposedPorts {
			eps = append(eps, ep.String())
		}
		cMap["ExposedPorts"] = eps
	}
	return json.Marshal(cMap)
}

func (ec *endpointConfiguration) UnmarshalJSON(b []byte) error {
	var (
		err  error
		cMap map[string]interface{}
		pms  []types.PortBinding
		eps  []types.TransportPort
	)
	if err = json.Unmarshal(b, &cMap); err != nil {
		return err
	}
	if _, ok := cMap["MacAddress"]; ok {
		if ec.MacAddress, err = net.ParseMAC(cMap["MacAddress"].(string)); err != nil {
			return types.InternalErrorf("failed to decode bridge endpoint configuration mac address after json unmarshal %s: %v", cMap["MacAddress"].(string), err)
		}
	}
	if _, ok := cMap["PortBindings"]; ok {
		for _, str := range cMap["PortBindings"].([]string) {
			pm := &types.PortBinding{}
			if err = pm.FromString(str); err != nil {
				return types.InternalErrorf("failed to decode bridge endpoint configuration port binding after json unmarshal %s: %v", str, err)
			}
			pms = append(pms, *pm)
		}
	}
	ec.PortBindings = pms

	if _, ok := cMap["ExposedPorts"]; ok {
		for _, str := range cMap["ExposedPorts"].([]string) {
			tp := &types.TransportPort{}
			if err = tp.FromString(str); err != nil {
				return types.InternalErrorf("failed to decode bridge endpoint configuration exposed port after json unmarshal %s: %v", str, err)
			}
			eps = append(eps, *tp)
		}
	}
	ec.ExposedPorts = eps
	return nil
}

func (epc *endpointConfiguration) CopyTo(dstEpc *endpointConfiguration) error {
	dstEpc.MacAddress = types.GetMacCopy(epc.MacAddress)
	dstEpc.PortBindings = make([]types.PortBinding, len(epc.PortBindings))
	copy(dstEpc.PortBindings, epc.PortBindings)
	dstEpc.ExposedPorts = make([]types.TransportPort, len(epc.ExposedPorts))
	copy(dstEpc.ExposedPorts, epc.ExposedPorts)
	return nil
}

