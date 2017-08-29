package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"runtime"

	"github.com/negbie/heplify/config"
	"github.com/negbie/heplify/decoder"
	"github.com/negbie/heplify/logp"
	"github.com/negbie/heplify/outputs"
	"github.com/negbie/heplify/sniffer"
	"github.com/tsg/gopacket"
	"github.com/tsg/gopacket/layers"
)

func InterfaceAddrsByName(ifaceName string) ([]string, error) {

	var buf []string

	i, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, err
	}
	addrs, err := i.Addrs()
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
			buf = append(buf, ip.String())
		case *net.IPAddr:
			ip = v.IP
			buf = append(buf, ip.String())
		}
	}
	return buf, nil
}

type MainWorker struct {
	publisher *outputs.Publisher
	decoder   *decoder.Decoder
}

func (mw *MainWorker) OnPacket(data []byte, ci *gopacket.CaptureInfo) {
	pkt, err := mw.decoder.Process(data, ci)
	// TODO check this
	if err != nil {
		panic(err)
	}
	if pkt != nil {
		mw.publisher.PublishEvent(pkt)
	}
}

func NewWorker(dl layers.LinkType) (sniffer.Worker, error) {
	var o outputs.Outputer
	var err error

	if config.Cfg.HepServer != "" {
		o, err = outputs.NewHepOutputer(config.Cfg.HepServer)
	} else {
		o, err = outputs.NewFileOutputer()
	}
	if err != nil {
		panic(err)
	}

	p := outputs.NewPublisher(o)
	d := decoder.NewDecoder()
	w := &MainWorker{publisher: p, decoder: d}
	return w, nil
}

func optParse() {

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s [option]\n", os.Args[0])
		flag.PrintDefaults()
	}

	var ifaceConfig config.InterfacesConfig
	var logging logp.Logging
	var fileRotator logp.FileRotator
	var rotateEveryKB uint64
	var keepFiles int

	flag.StringVar(&ifaceConfig.Device, "i", "any", "Listen on interface")
	flag.StringVar(&ifaceConfig.Type, "t", "af_packet", "Capture type are pcap or af_packet")
	flag.StringVar(&ifaceConfig.BpfFilter, "f", "", "BPF filter")
	flag.StringVar(&ifaceConfig.File, "rf", "", "Read packets from file")
	flag.StringVar(&ifaceConfig.Dumpfile, "df", "", "Dump to file")
	flag.IntVar(&ifaceConfig.Loop, "lp", 0, "Loop")
	flag.BoolVar(&ifaceConfig.WithVlans, "wl", false, "With vlans")
	flag.IntVar(&ifaceConfig.Snaplen, "s", 65535, "Snap length")
	flag.IntVar(&ifaceConfig.BufferSizeMb, "b", 128, "Interface buffer size (MB)")
	flag.StringVar(&logging.Level, "l", "info", "Logging level")
	flag.StringVar(&fileRotator.Path, "p", "", "Log path")
	flag.StringVar(&fileRotator.Name, "n", "heplify.log", "Log filename")
	flag.Uint64Var(&rotateEveryKB, "r", 51200, "The size (KB) of each log file")
	flag.IntVar(&keepFiles, "k", 4, "Keep the number of log files")
	flag.BoolVar(&config.Cfg.DoHep, "dh", true, "Use Hep")
	flag.StringVar(&config.Cfg.HepServer, "hs", "127.0.0.1:9060", "HepServer address")

	flag.Parse()

	config.Cfg.Iface = &ifaceConfig

	logging.Files = &fileRotator
	if logging.Files.Path != "" {
		tofiles := true
		logging.ToFiles = &tofiles

		rotateKB := rotateEveryKB * 1024
		logging.Files.RotateEveryBytes = &rotateKB
		logging.Files.KeepFiles = &keepFiles
	}
	config.Cfg.Logging = &logging

	if ifaceConfig.Device == "" && ifaceConfig.File == "" {
		flag.Usage()
		fmt.Println("no interface specified.")
		os.Exit(1)
	}

	if ifaceConfig.Device != "" {
		ifaceAddrs, err := InterfaceAddrsByName(config.Cfg.Iface.Device)
		if err != nil {
			flag.Usage()
			fmt.Println("error while looking up interface address.")
			os.Exit(1)
		}

		config.Cfg.IfaceAddrs = make(map[string]bool)
		for _, addr := range ifaceAddrs {
			config.Cfg.IfaceAddrs[addr] = true
		}
	}
}

func init() {
	optParse()
	logp.Init("heplify", config.Cfg.Logging)
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	capture := &sniffer.SnifferSetup{}
	capture.Init(false, config.Cfg.Iface.BpfFilter, NewWorker, config.Cfg.Iface)
	defer capture.Close()
	err := capture.Run()
	if err != nil {
		logp.Err("main capture %v", err)
	}
}
