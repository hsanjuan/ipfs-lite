package scp_conf

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
)

func initializeSSLedger(ssMainDir string) (string, error) {

	var err error
	if _, err = os.Stat(ssMainDir); os.IsNotExist(err) {
		err = os.Mkdir(ssMainDir, os.ModePerm)
	}
	if err != nil {
		return "", fmt.Errorf("Failed setting up main directory Err:%s", err.Error())
	}

	ssLedgerDir := ssMainDir + string(os.PathSeparator) + "ssLedger"

	if _, err = os.Stat(ssLedgerDir); os.IsNotExist(err) {
		err = os.Mkdir(ssLedgerDir, os.ModePerm)
	}
	if err != nil {
		return "", fmt.Errorf("Failed setting up ledger root dir Err:%s", err.Error())
	}

	return ssLedgerDir, nil
}

func New(
	h host.Host,
	root,
	deviceId,
	role string,
	mtdt map[string]interface{},
) (*ssConfig, error) {

	ledgerDir, err := initializeSSLedger(root)
	if err != nil {
		return nil, err
	}
	ep, err := time.Parse(time.RFC3339, Epoch)
	if err != nil {
		return nil, err
	}
	cyc, err := time.ParseDuration(CycleDuration)
	if err != nil {
		return nil, err
	}
	rt, err := strconv.ParseFloat(Rate, 64)
	if err != nil {
		return nil, err
	}
	return &ssConfig{
		userID:     h.ID(),
		role:       role,
		ledgerRoot: ledgerDir,
		epoch:      ep,
		cycle:      cyc,
		deviceId:   deviceId,
		rate:       rt,
		metadata:   mtdt,
	}, nil
}

type ssConfig struct {
	userID     peer.ID
	role       string
	ledgerRoot string
	deviceId   string
	epoch      time.Time
	cycle      time.Duration
	rate       float64
	metadata   map[string]interface{}
}

func (s *ssConfig) String() string {
	return fmt.Sprintf("%+v", s)
}

func (s *ssConfig) UserId() peer.ID {
	return s.userID
}

func (s *ssConfig) Role() string {
	return s.role
}

func (s *ssConfig) LedgerRoot() string {
	return s.ledgerRoot
}

func (s *ssConfig) Epoch() time.Time {
	return s.epoch
}

func (s *ssConfig) Cycle() time.Duration {
	return s.cycle
}

func (s *ssConfig) DeviceId() string {
	return s.deviceId
}

func (s *ssConfig) Rate() float64 {
	return s.rate
}

func (s *ssConfig) RawMetadata() map[string]interface{} {
	return s.metadata
}
