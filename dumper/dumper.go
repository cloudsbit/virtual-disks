package dumper

import (
	"fmt"
	"github.com/cloudsbit/virtual-disks/pkg/disklib"
	"github.com/cloudsbit/virtual-disks/pkg/virtual_disks"
	log "github.com/sirupsen/logrus"
	"os"
	"time"
)
const LIBRARY_PATH = "LD_LIBRARY_PATH"

type VddkVersion struct {
	Major   uint32
	Minor   uint32
	LibPath string
}

type ConnParams struct {
	VmMoRef              string `json:"VmMoRef"`
	VsphereHostName      string `json:"VsphereHostName"`
	VsphereHostPort      int64  `json:"VsphereHostPort"`
	VsphereUsername      string `json:"VsphereUsername"`
	VspherePassword      string `json:"VspherePassword"`
	VsphereThumbPrint    string `json:"VsphereThumbPrint"`
	VsphereSnapshotMoRef string `json:"VsphereSnapshotMoRef"`
}

type DiskParams struct {
	DiskPath     string `json:"diskPath"`
	DiskPathRoot string `json:"diskPathRoot"`
	ChangeId     string `json:"changeId"`
}

type ChangedArea struct {
	Start  int64 `json:"start"`
	Length int64 `json:"length"`
}

type DiskChangeInfo struct {
	StartOffset int64         `json:"startOffset"`
	Length      int64         `json:"length"`
	ChangedArea []ChangedArea `json:"changedArea"`
}

type CbtData struct {
	Conn   ConnParams     `json:"ConnParams"`
	Disk   DiskParams     `json:"DiskParams"`
	Change DiskChangeInfo `json:"DiskChangeInfo"`
}

type VadpDumper struct  {
	Identity string
	VddkVersion
	ConnParams
	//DiskParams
	//DiskChangeInfo
}

func GetThumbPrintForServer(host string, port string) (string, error) {
	return disklib.GetThumbPrintForServer(host, port)
}

func NewConnParams(host string, port int64, name string, password string, moRef string, snapRef string) (*ConnParams, error) {
	thumbPrint, err := GetThumbPrintForServer(host, string(port))
	if err != nil {
		log.Errorf("Thumbprint for %s:%s failed, err = %s\n", host, port, err)
		return nil, err
	}

	params := &ConnParams{
		VmMoRef: moRef,
		VsphereHostName: host,
		VsphereHostPort: port,
		VsphereUsername: name,
		VspherePassword: password,
		VsphereThumbPrint: thumbPrint,
		VsphereSnapshotMoRef: snapRef,
	}
	return params, nil
}

func NewVadpDumper(ver VddkVersion, conn ConnParams) (*VadpDumper, error) {
	identity := "rsb_dumper" + time.Now().String()
	dumper := &VadpDumper{ identity, ver, conn}
	return dumper, nil
}

func (d *VadpDumper) ConnectToDisk() error {
	if res := disklib.Init(d.Major, d.Minor, d.LibPath); res != nil {
		log.Errorf("disklib.Init: %v", res)
		return res
	}

	vmxSpec    := d.VmMoRef
	servName   := d.VsphereHostName
	thumbPrint := d.VsphereThumbPrint
	userName   := d.VsphereUsername
	password   := d.VspherePassword
	identity   := d.Identity
	path       := "[hp_stor] rsb_develop/rsb_develop.vmdk" // disk path
	flag       := uint32(disklib.VIXDISKLIB_FLAG_OPEN_READ_ONLY)
	snapRef    := d.VsphereSnapshotMoRef
	mode       := disklib.NBD

	params := disklib.NewConnectParams(
		vmxSpec,
		servName,
		thumbPrint,
		userName,
		password,
		"",
		"",
		"",
		"",
		identity,
		path,
		flag,
		false,
		snapRef,
		mode)

	_ = params
	return nil
}


func main() {
	vmLibPath := "/usr/local/vmware-vix-disklib-distrib/lib64"
	sysLibPath := os.Getenv(LIBRARY_PATH)
	os.Setenv(LIBRARY_PATH, fmt.Sprintf("%s:%s", vmLibPath, sysLibPath))

	var majorVersion uint32 = 7
	var minorVersion uint32 = 0

	if res := disklib.Init(majorVersion, minorVersion, vmLibPath); res != nil {
		log.Fatalf("disklib.Init: %v", res)
	}

	host := "192.168.1.100"
	port := "443"
	thumbPrint, err := disklib.GetThumbPrintForServer(host, port)
	if err != nil {
		log.Fatalf("Thumbprint for %s:%s failed, err = %s\n", host, port, err)
	}
	log.Printf("thumbprint: %v", thumbPrint)

	vmxSpec  := "moref=vm-972" // Dev-RSB
	userName := "administrator@vsphere.local"
	password := "Jrsa1234/"
	ds       := "datastore-944"
	identity := "RunStor-20210518"
	path     := "[hp_stor] rsb_develop/rsb_develop.vmdk" // disk path
	flag     := uint32(disklib.VIXDISKLIB_FLAG_OPEN_READ_ONLY)
	snapRef  := "snapshot-1102"
	mode     := disklib.NBD

	params := disklib.NewConnectParams(
		vmxSpec,
		host,
		thumbPrint,
		userName,
		password,
		"",
		ds,
		"",
		"",
		identity,
		path,
		flag,
		false,
		snapRef,
		mode)

	errVix := disklib.PrepareForAccess(params)
	if errVix != nil {
		fmt.Printf("Prepare for access failed. Error code: %d, error message: %s.\n", errVix.VixErrorCode(), errVix.Error())
		return
	}
	fmt.Printf("PrepareForAccess success\n")

	conn, errVix := disklib.ConnectEx(params)
	if errVix != nil {
		fmt.Printf("Connect to vixdisk lib failed. Error code: %d, error message: %s.\n", errVix.VixErrorCode(), errVix.Error())
		return
	}
	fmt.Printf("ConnectEx success\n")

	defer disklib.Disconnect(conn)
	defer disklib.EndAccess(params)

	dli, errVix := disklib.Open(conn, params)
	if errVix != nil {
		fmt.Printf("Open disk error. Error code: %d, error message: %s.\n", errVix.VixErrorCode(), errVix.Error())
		return
	}

	fmt.Printf("Open success\n")

	info, errVix := disklib.GetInfo(dli)
	if errVix != nil {
		fmt.Printf("Get disk info failed. Error code: %d, error message: %s.\n", errVix.VixErrorCode(), errVix.Error())
		return
	}
	fmt.Printf("Disk-Info: %+v\n", info)

	diskHandle := virtual_disks.NewDiskHandle(dli, conn, params, info)

	chunkSize  := 2048 // 1MB block size
	numSectors := diskHandle.Capacity() / disklib.VIXDISKLIB_SECTOR_SIZE
	fmt.Printf("Current Chunk info: chunkSize: %v, numSectors: %v, MAX: %v\n", chunkSize, numSectors, disklib.VIXDISKLIB_MAX_CHUNK_NUMBER)

	blockList, errVix := diskHandle.QueryAllocatedBlocks(0, disklib.VixDiskLibSectorType(numSectors), disklib.VixDiskLibSectorType(chunkSize))
	if errVix != nil {
		fmt.Printf("QueryAllocatedBlocks. Error code: %d, error message: %s.\n", errVix.VixErrorCode(), errVix.Error())
		return
	}
	fmt.Printf("Number of blocks: %d\n", len(blockList))
	fmt.Printf("Offset      Length\n")
	for _, ab := range blockList {
		fmt.Printf("%+v\n", ab)
	}

	//disklib.Disconnect(conn)
	//fmt.Printf("DisConnect success\n")

	//disklib.EndAccess(params)
	//fmt.Printf("EndAccess success\n")

	// 打开disk
	//diskReaderWriter, errVix := virtual_disks.Open(params, logrus.New())
	//if errVix != nil {
	//	disklib.EndAccess(params)
	//	fmt.Printf("Open failed, got error code: %d, error message: %s.", errVix.VixErrorCode(), errVix.Error())
	//	return
	//}

	//// QAB (assume at least 1GiB volume and 1MiB block size)
	//abInitial, errVix := diskReaderWriter.QueryAllocatedBlocks(0, 2048*1024, 2048)
	//if errVix != nil {
	//	fmt.Printf("QueryAllocatedBlocks failed: %d, error message: %s", errVix.VixErrorCode(), errVix.Error())
	//} else {
	//	fmt.Printf("Number of blocks: %d\n", len(abInitial))
	//	fmt.Printf("Offset      Length\n")
	//	for _, ab := range abInitial {
	//		fmt.Printf("0x%012x  0x%012x\n", ab.Offset(), ab.Length())
	//	}
	//}
	//diskReaderWriter.Close()

}
