package usrwg

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/edup2p/common/types"
	"github.com/edup2p/common/types/key"
	"golang.org/x/exp/maps"
	"golang.zx2c4.com/wireguard/conn"
)

type ToverSokBind struct {
	connMu     sync.RWMutex
	conns      map[key.NodePublic]*ChannelConn
	connChange chan bool

	endpointMu sync.RWMutex
	endpoints  map[key.NodePublic]*endpoint
}

func createBind() *ToverSokBind {
	return &ToverSokBind{
		conns:      make(map[key.NodePublic]*ChannelConn),
		connChange: make(chan bool),
		endpoints:  make(map[key.NodePublic]*endpoint),
	}
}

func (b *ToverSokBind) Open(uint16) (fns []conn.ReceiveFunc, fakePort uint16, err error) {
	fakePort = 12345
	fns = []conn.ReceiveFunc{b.ReadFromConns}

	return
}

func (b *ToverSokBind) Close() error {
	b.endpointMu.Lock()
	b.connMu.Lock()
	defer b.connMu.Unlock()
	defer b.endpointMu.Unlock()

	maps.Clear(b.endpoints)

	var errs []error

	for _, cc := range b.conns {
		// TODO log error
		if err := cc.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	maps.Clear(b.conns)

	if len(errs) > 0 {
		return fmt.Errorf("errors when closing connections: %w", errors.Join(errs...))
	}

	return nil
}

// ReadFromConns implements conn.ReceiveFunc
func (b *ToverSokBind) ReadFromConns(packets [][]byte, sizes []int, eps []conn.Endpoint) (n int, err error) {
	// We get a keys slice that could potentially get immediately outdated,
	// but we use it to fill buffers from existing conns first.
	b.connMu.RLock()
	keys := maps.Keys(b.conns)
	b.connMu.RUnlock()

fill:
	for i := 0; i < len(eps); i++ {
		var peer key.NodePublic
		var cc *ChannelConn
		var p []byte

		for {
			if len(keys) == 0 {
				break fill
			}

			peer, keys = keys[0], keys[1:]

			cc = b.fetchConn(peer)

			if cc == nil {
				continue
			}

			p = cc.tryGetOut()

			if p == nil {
				continue
			}

			break
		}

		eps[i] = b.endpointFor(peer)
		sizes[i] = len(p)
		copy(packets[i], p)

		n++
	}

	if n != 0 {
		// Buffer filled, return early
		return
	}

	// FIXME: Here we use reflect.Select, which may not be very good in the long run.
	// 	The above part handles the hot case, but this cold case can possibly have performance issues,
	//  as noted by several stackoverflow answers.
	//  The most important part is that it does not spinloop, and that it waits appropriately for any change.

	// Buffer wasn't filled, we wait for an incoming packet.
	p, ep := b.waitForValueFromConns()

	if p == nil {
		// Woken up by conn change, just return

		return
	}

	sizes[0] = len(p)
	copy(packets[0], p)
	eps[0] = ep

	n = 1

	return
}

func (b *ToverSokBind) waitForValueFromConns() ([]byte, *endpoint) {
	caseMap := b.buildConnsSelectCaseMap()
	connChangeCase := b.createConnChangeSelectCase()

	keys := maps.Keys(caseMap)

	cases := make([]reflect.SelectCase, len(caseMap))
	for i, k := range keys {
		cases[i] = caseMap[k]
	}

	cases = append(cases, connChangeCase)

	choice, recv, recvOk := reflect.Select(cases)

	slog.Log(
		context.Background(),
		types.LevelTrace,
		"waitForValueFromConns reflect.Select",
		"choice", choice,
		"len", len(cases),
		"recv", recv,
		"recvOk", recvOk,
		"cases", cases,
	)

	// choice == last index
	if choice == len(cases)-1 {
		return nil, nil
	}

	// We expect any other recv to be from slices
	return recv.Bytes(), b.endpointFor(keys[choice])
}

func (b *ToverSokBind) buildConnsSelectCaseMap() map[key.NodePublic]reflect.SelectCase {
	cases := make(map[key.NodePublic]reflect.SelectCase)

	b.connMu.RLock()
	defer b.connMu.RUnlock()

	for k, v := range b.conns {
		cases[k] = createChannelConnSelectCase(v)
	}

	return cases
}

func createChannelConnSelectCase(cc *ChannelConn) reflect.SelectCase {
	return reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(cc.outgoing),
		Send: reflect.Value{},
	}
}

func (b *ToverSokBind) createConnChangeSelectCase() reflect.SelectCase {
	return reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(b.connChange),
		Send: reflect.Value{},
	}
}

// SetMark is used by wireguard-go to avoid routing loops.
// TODO: double-check
func (b *ToverSokBind) SetMark(uint32) error {
	return nil
}

const SendTimeout = time.Second * 30

func (b *ToverSokBind) Send(bufs [][]byte, ep conn.Endpoint) error {
	e, ok := ep.(*endpoint)

	if !ok {
		return errors.New("wrong endpoint type")
	}

	cc := b.GetConn(e.k)

	for _, buf := range bufs {
		if !cc.putIn(buf, SendTimeout) {
			return errors.New("failed to send packet")
		}
	}

	return nil
}

func (b *ToverSokBind) ParseEndpoint(s string) (conn.Endpoint, error) {
	np, err := key.UnmarshalPublic(s)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal nodepublic: %w", err)
	}

	return b.endpointFor(*np), nil
}

func (b *ToverSokBind) BatchSize() int {
	return runtime.NumCPU()
}

func (b *ToverSokBind) GetConn(peer key.NodePublic) *ChannelConn {
	// Do a quick read to see if it exists
	cc := b.fetchConn(peer)

	if cc == nil {
		cc = b.createOrGetConn(peer)
	}

	return cc
}

func (b *ToverSokBind) fetchConn(peer key.NodePublic) *ChannelConn {
	b.connMu.RLock()
	defer b.connMu.RUnlock()

	return b.conns[peer]
}

func (b *ToverSokBind) createOrGetConn(peer key.NodePublic) *ChannelConn {
	b.connMu.Lock()
	defer b.connMu.Unlock()

	cc, ok := b.conns[peer]

	if !ok {
		cc = makeChannelConn()
		b.conns[peer] = cc

		b.notifyConnChange()
	}

	return cc
}

func (b *ToverSokBind) CloseConn(peer key.NodePublic) {
	b.connMu.Lock()
	defer b.connMu.Unlock()

	cc, ok := b.conns[peer]
	if ok {
		if err := cc.Close(); err != nil {
			slog.Error("failed to close channel", "peer", peer, "err", err)
		}
	}

	delete(b.conns, peer)

	b.notifyConnChange()
}

func (b *ToverSokBind) notifyConnChange() {
	select {
	case b.connChange <- true:
	default:
	}
}

func (b *ToverSokBind) endpointFor(peer key.NodePublic) *endpoint {
	// FIXME: at no point do we deallocate endpoints while the device is open, being a potential (very slow) memory leak.
	//  however, there is also not a clear mechanism or way to detect that wggo is not using the endpoint anymore,
	//  its opaque.

	e := b.fetchEndpoint(peer)

	if e == nil {
		b.endpointMu.Lock()
		defer b.endpointMu.Unlock()

		e = &endpoint{k: peer}

		b.endpoints[peer] = e
	}

	return e
}

func (b *ToverSokBind) fetchEndpoint(peer key.NodePublic) *endpoint {
	b.endpointMu.RLock()
	defer b.endpointMu.RUnlock()

	return b.endpoints[peer]
}
