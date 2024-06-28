package network

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/celestiaorg/celestia-app/app"
	"github.com/celestiaorg/celestia-app/app/encoding"
	testgroundconsts "github.com/celestiaorg/celestia-app/pkg/appconsts/testground"
	"github.com/celestiaorg/celestia-app/test/util/genesis"
	blobtypes "github.com/celestiaorg/celestia-app/x/blob/types"
	srvconfig "github.com/cosmos/cosmos-sdk/server/config"
	tmconfig "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/consensus"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/pkg/trace/schema"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	"github.com/testground/sdk-go/runtime"
)

// buff-7 just had the envelope buffers enabled
// buff-9 also had a change in the read buffer size
// buff-11 had the blocker from stats channel removed
// buff-12 has the extra trace for the buffer sizes
// buff-13 has the change to marking all peers as good
// "mut-1 has the mutex contention fix"
// "mb-2" buffers and mutex fixes

func init() {
	consensus.UseWAL = true
	node.PushMetrics = false
	node.PushGateWayURL = "http://51.159.176.205:9191"
	consensus.DataChannelPriority = 10
	// consensus.EnvelopeBuffer = 10000
	consensus.DataChannelCapacity = 100
	// p2p.UseBufferedReceives = true
	// conn.MinReadBufferSize = 1024 * 1024 * 32
	// conn.SetDataMaxSize(2048)
	// conn.MinWriteBufferSize = 65536
	// conn.NumBatchPacketMsgs = 10
	// p2p.TCPSocketReadBuffer = 1024 * 64
	// p2p.TCPSocketWriteBuffer = 1024 * 64
	// p2p.SetTCPBuffers = false
}

const (
	TimeoutParam           = "timeout"
	ChainIDParam           = "chain_id"
	ValidatorsParam        = "validators"
	FullNodesParam         = "full_nodes"
	HaltHeightParam        = "halt_height"
	PexParam               = "pex"
	SeedNodeParam          = "seed_node"
	BlobSequencesParam     = "blob_sequences"
	BlobSizesParam         = "blob_sizes"
	BlobsPerSeqParam       = "blobs_per_sequence"
	TimeoutCommitParam     = "timeout_commit"
	TimeoutProposeParam    = "timeout_propose"
	InboundPeerCountParam  = "inbound_peer_count"
	OutboundPeerCountParam = "outbound_peer_count"
	GovMaxSquareSizeParam  = "gov_max_square_size"
	MaxBlockBytesParam     = "max_block_bytes"
	MempoolParam           = "mempool"
	BroadcastTxsParam      = "broadcast_txs"
	TracingTokenParam      = "tracing_token"
	TracingURLParam        = "tracing_url"
	TracingNodesParam      = "tracing_nodes"
	ExperimentParam        = "experiment"
)

type Params struct {
	ChainID           string
	Validators        int
	FullNodes         int
	HaltHeight        int
	Timeout           time.Duration
	Pex               bool
	Configurators     []Configurator
	GenesisModifiers  []genesis.Modifier
	PerPeerBandwidth  int
	BlobsPerSeq       int
	BlobSequences     int
	BlobSizes         int
	InboundPeerCount  int
	OutboundPeerCount int
	GovMaxSquareSize  int
	MaxBlockBytes     int
	TimeoutCommit     time.Duration
	TimeoutPropose    time.Duration
	Mempool           string
	BroadcastTxs      bool
	TracingParams
	Experiment string
}

type TracingParams struct {
	Nodes int
	URL   string
	Token string
}

func ParseTracingParams(runenv *runtime.RunEnv) TracingParams {
	return TracingParams{
		Nodes: runenv.IntParam(TracingNodesParam),
		URL:   "http://51.158.232.250:8086",
		Token: "SgmlSaqxiR6ZTmBhyR5E0C9Nf_x35AoxeLyn4NE5jYBlMFIPDHmNBE_levqq4UBnjfoJXXYYxkha7F3GUWki9w==",
	}
}

func ParseParams(ecfg encoding.Config, runenv *runtime.RunEnv) (*Params, error) {
	var err error
	p := &Params{}

	p.ChainID = runenv.StringParam(ChainIDParam)

	PerPeerBandwidth, err := parseBandwidth(runenv.StringParam("per_peer_bandwidth"))
	if err != nil {
		return nil, err
	}
	p.PerPeerBandwidth = int(PerPeerBandwidth)

	p.Validators = runenv.IntParam(ValidatorsParam)

	p.FullNodes = runenv.IntParam(FullNodesParam)

	p.HaltHeight = runenv.IntParam(HaltHeightParam)

	p.BlobSequences = runenv.IntParam(BlobSequencesParam)

	p.BlobSizes = runenv.IntParam(BlobSizesParam)

	p.BlobsPerSeq = runenv.IntParam(BlobsPerSeqParam)

	p.InboundPeerCount = runenv.IntParam(InboundPeerCountParam)

	p.OutboundPeerCount = runenv.IntParam(OutboundPeerCountParam)

	p.GovMaxSquareSize = runenv.IntParam(GovMaxSquareSizeParam)

	p.MaxBlockBytes = runenv.IntParam(MaxBlockBytesParam)

	p.Timeout, err = time.ParseDuration(runenv.StringParam(TimeoutParam))
	if err != nil {
		return nil, err
	}

	p.TimeoutCommit, err = time.ParseDuration(runenv.StringParam(TimeoutCommitParam))
	if err != nil {
		return nil, err
	}

	p.TimeoutPropose, err = time.ParseDuration(runenv.StringParam(TimeoutProposeParam))
	if err != nil {
		return nil, err
	}

	p.Configurators, err = GetConfigurators(runenv)
	if err != nil {
		return nil, err
	}

	p.GenesisModifiers = p.getGenesisModifiers(ecfg)

	p.Pex = runenv.BooleanParam(PexParam)

	p.Mempool = runenv.StringParam(MempoolParam)

	p.BroadcastTxs = runenv.BooleanParam(BroadcastTxsParam)

	p.TracingParams = ParseTracingParams(runenv)

	p.Experiment = runenv.StringParam(ExperimentParam)

	return p, p.ValidateBasic()
}

func (p *Params) ValidateBasic() error {
	if p.Validators < 1 {
		return errors.New("invalid number of validators")
	}
	if p.FullNodes < 0 {
		return errors.New("invalid number of full nodes")
	}

	return nil
}

func (p *Params) NodeCount() int {
	return p.FullNodes + p.Validators
}

func TracingTables() []string {
	return schema.AllTables()
}

func StandardCometConfig(params *Params) *tmconfig.Config {
	cmtcfg := app.DefaultConsensusConfig()
	cmtcfg.Instrumentation.PrometheusListenAddr = ":26660"
	cmtcfg.Instrumentation.Prometheus = false
	cmtcfg.P2P.PexReactor = params.Pex
	cmtcfg.P2P.SendRate = int64(params.PerPeerBandwidth)
	cmtcfg.P2P.RecvRate = int64(params.PerPeerBandwidth)
	cmtcfg.P2P.AddrBookStrict = false
	cmtcfg.Consensus.TimeoutCommit = params.TimeoutCommit
	cmtcfg.Consensus.TimeoutPropose = params.TimeoutPropose
	cmtcfg.TxIndex.Indexer = "kv"
	cmtcfg.Mempool.Broadcast = params.BroadcastTxs
	cmtcfg.Mempool.Version = params.Mempool
	cmtcfg.Mempool.MaxTxsBytes = 1_000_000_000
	cmtcfg.Mempool.MaxTxBytes = 1_000_000_000
	cmtcfg.Mempool.TTLNumBlocks = 100
	cmtcfg.Mempool.TTLDuration = 40 * time.Minute
	cmtcfg.Mempool.MaxGossipDelay = 20 * time.Second
	cmtcfg.Instrumentation.TraceType = "local"
	cmtcfg.Instrumentation.TracePushConfig = "s3.json"
	cmtcfg.Instrumentation.TraceBufferSize = 5000
	cmtcfg.Instrumentation.TracingTables = strings.Join(TracingTables(), ",")
	cmtcfg.Instrumentation.TracePullAddress = ""
	return cmtcfg
}

func StandardAppConfig(_ *Params) *srvconfig.Config {
	return app.DefaultAppConfig()
}

func TestgroundConsensusParams(params *Params) *tmproto.ConsensusParams {
	cp := app.DefaultConsensusParams()
	cp.Block.MaxBytes = int64(params.MaxBlockBytes)
	cp.Version.AppVersion = testgroundconsts.Version
	return cp
}

func peerID(ip string, networkKey ed25519.PrivKey) string {
	nodeID := string(p2p.PubKeyToID(networkKey.PubKey()))
	return fmt.Sprintf("%s@%s:26656", nodeID, ip)
}

func (p *Params) getGenesisModifiers(ecfg encoding.Config) []genesis.Modifier {
	var modifiers []genesis.Modifier

	blobParams := blobtypes.DefaultParams()
	blobParams.GovMaxSquareSize = uint64(p.GovMaxSquareSize)
	modifiers = append(modifiers, genesis.SetBlobParams(ecfg.Codec, blobParams))

	modifiers = append(modifiers, genesis.ImmediateProposals(ecfg.Codec))

	return modifiers
}

// parseBandwidth is a crude helper function to parse bandwidth strings. For
// example Kib, Kb, or KB are all valid units. Kb and KB are treated as 1000.
// Kib is 1024.
func parseBandwidth(s string) (uint64, error) {
	var multiplier uint64

	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "Kib") {
		multiplier = 1 << 10
	} else if strings.HasSuffix(s, "Mib") {
		multiplier = 1 << 20
	} else if strings.HasSuffix(s, "Gib") {
		multiplier = 1 << 30
	} else if strings.HasSuffix(s, "Tib") {
		multiplier = 1 << 40
	} else if strings.HasSuffix(s, "Kb") || strings.HasSuffix(s, "KB") {
		multiplier = 1000
	} else if strings.HasSuffix(s, "Mb") || strings.HasSuffix(s, "MB") {
		multiplier = 1000 * 1000
	} else if strings.HasSuffix(s, "Gb") || strings.HasSuffix(s, "GB") {
		multiplier = 1000 * 1000 * 1000
	} else if strings.HasSuffix(s, "Tb") || strings.HasSuffix(s, "TB") {
		multiplier = 1000 * 1000 * 1000 * 1000
	} else {
		return 0, fmt.Errorf("unknown unit in string: %s", s)
	}

	numberStr := strings.TrimRight(s, "KMGTib")
	number, err := strconv.ParseFloat(numberStr, 64)
	if err != nil {
		return 0, err
	}

	return uint64(number * float64(multiplier)), nil
}
