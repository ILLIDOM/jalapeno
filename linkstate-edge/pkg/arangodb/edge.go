package arangodb

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	driver "github.com/arangodb/go-driver"
	"github.com/golang/glog"
	"github.com/jalapeno/lslinknode-edge/pkg/kafkanotifier"
	notifier "github.com/jalapeno/topology/pkg/kafkanotifier"
	"github.com/sbezverk/gobmp/pkg/base"
	"github.com/sbezverk/gobmp/pkg/message"
	"github.com/sbezverk/gobmp/pkg/sr"
	"github.com/sbezverk/gobmp/pkg/srv6"
)

const LSNodeEdgeCollection = "LSNode_Edge"

var Notifier *kafkanotifier.Notifier;

func InitializeKafkaNotifier(msgSrvAddr string) {
	kNotifier, err := kafkanotifier.NewKafkaNotifier(msgSrvAddr)
	if err != nil {
		glog.Errorf("failed to initialize events notifier with error: %+v", err)
		os.Exit(1)
	}
	Notifier = kNotifier
}

func (a *arangoDB) lsLinkHandler(obj *notifier.EventMessage) error {
	ctx := context.TODO()
	if obj == nil {
		return fmt.Errorf("event message is nil")
	}
	// Check if Collection encoded in ID exists
	c := strings.Split(obj.ID, "/")[0]
	if strings.Compare(c, a.edge.Name()) != 0 {
		return fmt.Errorf("configured collection name %s and received in event collection name %s do not match", a.edge.Name(), c)
	}
	glog.V(5).Infof("Processing action: %s for key: %s ID: %s", obj.Action, obj.Key, obj.ID)
	var o message.LSLink
	_, err := a.edge.ReadDocument(ctx, obj.Key, &o)
	if err != nil {
		// In case of a LSLink removal notification, reading it will return Not Found error
		if !driver.IsNotFound(err) {
			return fmt.Errorf("failed to read existing document %s with error: %+v", obj.Key, err)
		}
		// If operation matches to "del" then it is confirmed delete operation, otherwise return error
		if obj.Action != "del" {
			return fmt.Errorf("document %s not found but Action is not \"del\", possible stale event", obj.Key)
		}
		return a.processEdgeRemoval(ctx, obj.Key, obj.Action)
	}
	switch obj.Action {
	case "add":
		fallthrough
	case "update":
		if err := a.processEdge(ctx, obj.Key, &o); err != nil {
			return fmt.Errorf("failed to process action %s for edge %s with error: %+v", obj.Action, obj.Key, err)
		}
	}

	return nil
}

func (a *arangoDB) lsNodeHandler(obj *notifier.EventMessage) error {
	ctx := context.TODO()
	if obj == nil {
		return fmt.Errorf("event message is nil")
	}
	// Check if Collection encoded in ID exists
	c := strings.Split(obj.ID, "/")[0]
	if strings.Compare(c, a.vertex.Name()) != 0 {
		return fmt.Errorf("configured collection name %s and received in event collection name %s do not match", a.vertex.Name(), c)
	}
	glog.V(5).Infof("Processing action: %s for key: %s ID: %s", obj.Action, obj.Key, obj.ID)
	var o message.LSNode
	_, err := a.vertex.ReadDocument(ctx, obj.Key, &o)
	if err != nil {
		// In case of a LSNode removal notification, reading it will return Not Found error
		if !driver.IsNotFound(err) {
			return fmt.Errorf("failed to read existing document %s with error: %+v", obj.Key, err)
		}
		// If operation matches to "del" then it is confirmed delete operation, otherwise return error
		if obj.Action != "del" {
			return fmt.Errorf("document %s not found but Action is not \"del\", possible stale event", obj.Key)
		}
		return a.processVertexRemoval(ctx, obj.Key, obj.Action)
	}
	switch obj.Action {
	case "add":
		fallthrough
	case "update":
		if err := a.processVertex(ctx, obj.Key, &o); err != nil {
			return fmt.Errorf("failed to process action %s for vertex %s with error: %+v", obj.Action, obj.Key, err)
		}
	}

	return nil
}

type lsNodeEdgeObject struct {
	Key           string                `json:"_key"`
	From          string                `json:"_from"`
	To            string                `json:"_to"`
	Link          string                `json:"link"`
	ProtocolID    base.ProtoID          `json:"protocol_id"`
	DomainID      int64                 `json:"domain_id"`
	MTID          uint16                `json:"mt_id"`
	AreaID        string                `json:"area_id"`
	LocalLinkID   uint32                `json:"local_link_id"`
	RemoteLinkID  uint32                `json:"remote_link_id"`
	LocalLinkIP   string                `json:"local_link_ip"`
	RemoteLinkIP  string                `json:"remote_link_ip"`
	LocalNodeASN  uint32                `json:"local_node_asn"`
	RemoteNodeASN uint32                `json:"remote_node_asn"`
	SRv6ENDXSID   []*srv6.EndXSIDTLV    `json:"srv6_endx_sid"`
	LSAdjSID      []*sr.AdjacencySIDTLV `json:"ls_adj_sid"`
}

// processEdge processes a single LS Link connection which is a unidirectional edge between two nodes (vertices).
func (a *arangoDB) processEdge(ctx context.Context, key string, e *message.LSLink) error {
	if e.ProtocolID == base.BGP {
		return nil
	}
	ln, err := a.getNode(ctx, e, true)
	if err != nil {
		return err
	}

	rn, err := a.getNode(ctx, e, false)
	if err != nil {
		return err
	}
	glog.V(6).Infof("Local node -> Protocol: %+v Domain ID: %+v IGP Router ID: %+v",
		ln.ProtocolID, ln.DomainID, ln.IGPRouterID)
	glog.V(6).Infof("Remote node -> Protocol: %+v Domain ID: %+v IGP Router ID: %+v",
		rn.ProtocolID, rn.DomainID, rn.IGPRouterID)

	mtid := 0
	if e.MTID != nil {
		mtid = int(e.MTID.MTID)
	}
	ne := lsNodeEdgeObject{
		Key:           key,
		From:          ln.ID,
		To:            rn.ID,
		Link:          e.Key,
		ProtocolID:    e.ProtocolID,
		DomainID:      e.DomainID,
		MTID:          uint16(mtid),
		AreaID:        e.AreaID,
		LocalLinkID:   e.LocalLinkID,
		RemoteLinkID:  e.RemoteLinkID,
		LocalLinkIP:   e.LocalLinkIP,
		RemoteLinkIP:  e.RemoteLinkIP,
		LocalNodeASN:  e.LocalNodeASN,
		RemoteNodeASN: e.RemoteNodeASN,
		SRv6ENDXSID:   e.SRv6ENDXSID,
		LSAdjSID:      e.LSAdjacencySID,
	}
	if _, err := a.graph.CreateDocument(ctx, &ne); err != nil {
		if !driver.IsConflict(err) {
			return err
		}
		// The document already exists, updating it with the latest info
		if _, err := a.graph.UpdateDocument(ctx, ne.Key, &ne); err != nil {
			return err
		}
	}

	return nil
}

func (a *arangoDB) processVertex(ctx context.Context, key string, ln *message.LSNode) error {
	if ln.ProtocolID == 7 {
		return nil
	}
	// Check if there is an edge with matching to LSNode's e.IGPRouterID, e.AreaID, e.DomainID and e.ProtocolID
	query := "FOR d IN " + a.edge.Name() +
		" filter d.igp_router_id == " + "\"" + ln.IGPRouterID + "\"" +
		" filter d.domain_id == " + strconv.Itoa(int(ln.DomainID)) +
		" filter d.protocol_id == " + strconv.Itoa(int(ln.ProtocolID))
	// If OSPFv2 or OSPFv3, then query must include AreaID
	if ln.ProtocolID == base.OSPFv2 || ln.ProtocolID == base.OSPFv3 {
		query += " filter d.area_id == " + "\"" + ln.AreaID + "\""
	}
	query += " return d"
	lcursor, err := a.db.Query(ctx, query, nil)
	if err != nil {
		return err
	}
	defer lcursor.Close()
	var l message.LSLink
	// Processing each LSLink
	i := 0
	for ; ; i++ {
		_, err := lcursor.ReadDocument(ctx, &l)
		if err != nil {
			if !driver.IsNoMoreDocuments(err) {
				return err
			}
			break
		}
		rn, err := a.getNode(ctx, &l, false)
		if err != nil {
			continue
		}
		glog.V(6).Infof("Local node -> Protocol: %+v Domain ID: %+v IGP Router ID: %+v",
			ln.ProtocolID, ln.DomainID, ln.IGPRouterID)
		glog.V(6).Infof("Remote node -> Protocol: %+v Domain ID: %+v IGP Router ID: %+v",
			rn.ProtocolID, rn.DomainID, rn.IGPRouterID)

		mtid := 0
		if ln.MTID != nil {
			mtid = int(l.MTID.MTID)
		}
		ne := lsNodeEdgeObject{
			Key:  l.Key,
			From: ln.ID,
			To:   rn.ID,
			MTID: uint16(mtid),
			Link: l.Key,
		}
		if _, err := a.graph.CreateDocument(ctx, &ne); err != nil {
			if !driver.IsConflict(err) {
				return err
			}
			// The document already exists, updating it with the latest info
			if _, err := a.graph.UpdateDocument(ctx, ne.Key, &ne); err != nil {
				return err
			}
		}
	}

	return nil
}

// processEdgeRemoval removes a record from Node's graph collection
// since the key matches in both collections (LS Links and Nodes' Graph) deleting the record directly.
func (a *arangoDB) processEdgeRemoval(ctx context.Context, key string, action string) error {
	doc, err := a.graph.RemoveDocument(ctx, key)
	if err != nil {
		if !driver.IsNotFound(err) {
			return err
		}
		return nil
	}
	notifyKafka(doc, action)
	return nil
}

// processEdgeRemoval removes all documents where removed Vertix (LSNode) is referenced in "_to" or "_from"
func (a *arangoDB) processVertexRemoval(ctx context.Context, key string, action string) error {
	query := "FOR d IN " + a.graph.Name() +
		" filter d._to == " + "\"" + key + "\"" + " OR" + " d._from == " + "\"" + key + "\"" +
		" return d"
	cursor, err := a.db.Query(ctx, query, nil)
	if err != nil {
		return err
	}
	defer cursor.Close()

	for {
		var p lsNodeEdgeObject
		_, err := cursor.ReadDocument(ctx, &p)
		if err != nil {
			if !driver.IsNoMoreDocuments(err) {
				return err
			}
			break
		}
		glog.V(6).Infof("Removing from %s object %s", a.graph.Name(), p.Key)
		var doc driver.DocumentMeta
		if doc, err = a.graph.RemoveDocument(ctx, p.Key); err != nil {
			if !driver.IsNotFound(err) {
				return err
			}
			return nil
		}
	}
	return nil
}

func (a *arangoDB) getNode(ctx context.Context, e *message.LSLink, local bool) (*message.LSNode, error) {
	// Need to fine Node object matching LS Link's IGP Router ID
	query := "FOR d IN " + a.vertex.Name()
	if local {
		query += " filter d.igp_router_id == " + "\"" + e.IGPRouterID + "\""
	} else {
		query += " filter d.igp_router_id == " + "\"" + e.RemoteIGPRouterID + "\""
	}
	query += " filter d.domain_id == " + strconv.Itoa(int(e.DomainID)) +
		" filter d.protocol_id == " + strconv.Itoa(int(e.ProtocolID))
	// If OSPFv2 or OSPFv3, then query must include AreaID
	if e.ProtocolID == base.OSPFv2 || e.ProtocolID == base.OSPFv3 {
		query += " filter d.area_id == " + "\"" + e.AreaID + "\""
	}
	query += " return d"
	lcursor, err := a.db.Query(ctx, query, nil)
	if err != nil {
		return nil, err
	}
	defer lcursor.Close()
	var ln message.LSNode
	// var lm driver.DocumentMeta
	i := 0
	for ; ; i++ {
		_, err := lcursor.ReadDocument(ctx, &ln)
		if err != nil {
			if !driver.IsNoMoreDocuments(err) {
				return nil, err
			}
			break
		}
	}
	if i == 0 {
		return nil, fmt.Errorf("query %s returned 0 results", query)
	}
	if i > 1 {
		return nil, fmt.Errorf("query %s returned more than 1 result", query)
	}

	return &ln, nil
}
