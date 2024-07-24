package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/celestiaorg/celestia-app/v3/test/e2e/testnet"
	"github.com/tendermint/tendermint/rpc/client/http"
)

const (
	compactBlocksVersion = "a77609b"
)

func main() {
	if err := Run(); err != nil {
		log.Fatalf("failed to run experiment: %v", err)
	}
}

func Run() error {
	const (
		nodes          = 5
		timeoutCommit  = 5 * time.Second
		timeoutPropose = 4 * time.Second
		version        = compactBlocksVersion
	)

	network, err := testnet.New("compact-blocks", 864, nil, "test")
	if err != nil {
		return err
	}
	defer network.Cleanup()

	err = network.CreateGenesisNodes(nodes, version, 10000000, 0, testnet.DefaultResources)
	if err != nil {
		return err
	}

	gRPCEndpoints, err := network.RemoteGRPCEndpoints()
	if err != nil {
		return err
	}

	for _, node := range network.Nodes() {
		if err := node.Instance.EnableBitTwister(); err != nil {
			return fmt.Errorf("failed to enable bit twister: %v", err)
		}
	}

	err = network.CreateTxClients(
		compactBlocksVersion,
		5,
		"1000-8000",
		1,
		testnet.DefaultResources,
		gRPCEndpoints[:2],
	)
	if err != nil {
		return err
	}

	log.Printf("Setting up network\n")
	err = network.Setup(
		testnet.WithTimeoutCommit(timeoutCommit),
		testnet.WithTimeoutPropose(timeoutPropose),
		testnet.WithMempool("v2"),
	)
	if err != nil {
		return err
	}

	log.Printf("Starting network\n")
	err = network.Start()
	if err != nil {
		return err
	}

	for _, node := range network.Nodes() {
		err = node.Instance.SetLatencyAndJitter(100, 10)
		if err != nil {
			return err
		}
	}

	// run the test for 5 minutes
	heightTicker := time.NewTicker(20 * time.Second)
	timeout := time.NewTimer(5 * time.Minute)
	client, err := network.Node(0).Client()
	if err != nil {
		return err
	}
	for {
		select {
		case <-heightTicker.C:
			status, err := client.Status(context.Background())
			if err != nil {
				log.Printf("Error getting status: %v", err)
				continue
			}
			log.Printf("Height: %v", status.SyncInfo.LatestBlockHeight)

		case <-timeout.C:
			network.StopTxClients()
			log.Println("--- COLLECTING DATA")
			if err := saveBlockTimes(network); err != nil {
				log.Printf("Error saving block times: %v", err)
			}
			log.Println("--- FINISHED ✅: Compact Blocks")
			return nil
		}
	}
}

func saveBlockTimes(testnet *testnet.Testnet) error {
	file, err := os.Create(fmt.Sprintf("%s-%s-block-times.csv", time.Now().Format("2006-01-02-15-04-05"), testnet.Node(0).Version))
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	err = writer.Write([]string{"height", "block time", "block size", "last commit round"})
	if err != nil {
		return err
	}

	nodes := testnet.Nodes()
	clients := make([]*http.HTTP, len(nodes))
	for i, node := range nodes {
		clients[i], err = node.Client()
		if err != nil {
			return err
		}
	}
	status, err := clients[0].Status(context.Background())
	if err != nil {
		return err
	}
	index := 0
	for height := status.SyncInfo.EarliestBlockHeight; height <= status.SyncInfo.LatestBlockHeight; height++ {
		resp, err := clients[index].Block(context.Background(), &height)
		if err != nil {
			log.Printf("Error getting header for height %d: %v", height, err)
			index++
			if index == len(nodes) {
				return fmt.Errorf("all nodes failed to get header for height %d", height)
			}
			// retry the height
			height--
			continue
		}
		blockSize := 0
		for _, tx := range resp.Block.Txs {
			blockSize += len(tx)
		}
		err = writer.Write([]string{fmt.Sprintf("%d", height), fmt.Sprintf("%d", resp.Block.Time.UnixNano()), fmt.Sprintf("%d", blockSize), fmt.Sprintf("%d", resp.Block.LastCommit.Round)})
		if err != nil {
			return err
		}
	}
	return nil
}
