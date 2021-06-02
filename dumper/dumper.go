package dumper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/cloudsbit/virtual-disks/pkg/disklib"
	"github.com/cloudsbit/virtual-disks/pkg/virtual_disks"
	log "github.com/sirupsen/logrus"
	"strconv"
	"time"
)

type VddkVersion struct {
	Major   uint32
	Minor   uint32
	LibPath string
}

type ConnParams struct {
	VmMoRef              string `json:"VmMoRef"`
	VsphereHostName      string `json:"VsphereHostName"`
	VsphereHostPort      int    `json:"VsphereHostPort"`
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

type DumpMode int
const (
	DumpBlocks = iota
	DumpBackup
	DumpClone
	DumpResotre
)

type VddkParams struct  {
	Identity string
	VddkVersion
	ConnParams
	DiskParams
	//DiskChangeInfo
}

type VadpDumper struct  {
	VddkParams
	dumpMode   DumpMode

	connParams *disklib.ConnectParams
	connection *disklib.VixDiskLibConnection
	diskInfo   *disklib.VixDiskLibInfo
	diskHandle *virtual_disks.DiskConnectHandle

	ChangeInfo *DiskChangeInfo

	//lConnParams *disklib.ConnectParams
	lConnection *disklib.VixDiskLibConnection
	writeHandle *virtual_disks.DiskConnectHandle
}

func ParseCbtData(conf string) (*CbtData, error) {
	cbtData := &CbtData{}
	if err := json.Unmarshal([]byte(conf), cbtData); err != nil {
		//FIXME:
		//return nil, err
	}
	return cbtData, nil
}

func GetThumbPrintForServer(host string, port int) (string, error) {
	strPort := strconv.FormatInt(int64(port), 10)
	return disklib.GetThumbPrintForServer(host, strPort)
}

func getDiskLibFlag(mode DumpMode) uint32 {
	// only for vsphere VM
	flag := 0
	if mode == DumpBlocks || mode == DumpBackup {
		flag |= disklib.VIXDISKLIB_FLAG_OPEN_READ_ONLY
	}
	return uint32(flag)
}

func isReadOnly(mode DumpMode) bool {
	// only for vsphere VM
	if mode == DumpResotre {
		return false
	}
	return true
}

func NewConnParams(host string, port int, name string, password string, moRef string, snapRef string) (*ConnParams, error) {
	thumbPrint, err := GetThumbPrintForServer(host, port)
	if err != nil {
		log.Errorf("Thumbprint for %v:%v failed, err = %v\n", host, port, err)
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

func NewVddkParams(ver VddkVersion, conn ConnParams, disk DiskParams) (*VddkParams, error) {
	identity := fmt.Sprintf("%v_%v", "rsb_dumper", time.Now().Second())
	params := &VddkParams{ identity, ver, conn, disk}
	return params, nil
}

func NewVadpDumper(params VddkParams, mode DumpMode) (*VadpDumper, error) {
	connParams  := new(disklib.ConnectParams)
	connection  := new(disklib.VixDiskLibConnection)
	diskInfo    := new(disklib.VixDiskLibInfo)
	diskHandle  := new(virtual_disks.DiskConnectHandle)
	changeInfo  := new(DiskChangeInfo)
	lConnection := new(disklib.VixDiskLibConnection)
	writeHandle := new(virtual_disks.DiskConnectHandle)

	dumper := &VadpDumper{
		params,
		mode,
		connParams,
		connection,
		  diskInfo,
		diskHandle,
		changeInfo,
		lConnection,
		writeHandle,
		}
	return dumper, nil
}

func (d *VadpDumper) ConnectToDisk() (err error) {
	if res := disklib.Init(d.Major, d.Minor, d.LibPath); res != nil {
		return fmt.Errorf("disklib.Init: %v", res)
	}

	vmxSpec    := d.VmMoRef
	servName   := d.VsphereHostName
	thumbPrint := d.VsphereThumbPrint
	userName   := d.VsphereUsername
	password   := d.VspherePassword
	identity   := d.Identity
	path       := d.DiskPathRoot
	flag       := getDiskLibFlag(d.dumpMode)
	readOnly   := isReadOnly(d.dumpMode)
	snapRef    := d.VsphereSnapshotMoRef
	mode       := disklib.NBD // FIXME

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
		readOnly,
		snapRef,
		mode)
	d.connParams = &params
	log.Infof("ConnectParams: %v", params)

	errVix := disklib.PrepareForAccess(params)
	if errVix != nil {
		return fmt.Errorf("PrepareForAccess: %v", errVix)
	}
	log.Infof("PrepareForAccess success\n")

	defer func() {
		if err != nil {
			disklib.EndAccess(params)
		}
	}()

	conn, errVix := disklib.ConnectEx(params)
	if errVix != nil {
		return fmt.Errorf("ConnectEx: %v", errVix)
	}
	log.Infof("ConnectEx success\n")
	d.connection = &conn

	defer func() {
		if err != nil {
			disklib.Disconnect(conn)
		}
	}()

	dli, errVix := disklib.Open(conn, params)
	if errVix != nil {
		return fmt.Errorf("Open: %v", errVix)
	}
	log.Infof("Open success\n")

	info, errVix := disklib.GetInfo(dli)
	if errVix != nil {
		return fmt.Errorf("GetInfo: %v", errVix)
	}
	log.Infof("GetInfo: %+v\n", info)
	d.diskInfo = &info


	diskHandle := virtual_disks.NewDiskHandle(dli, conn, params, info)

	if d.dumpMode == DumpResotre {
		d.writeHandle = &diskHandle
	} else {
		d.diskHandle = &diskHandle
	}

	d.ChangeInfo.StartOffset = 0
	d.ChangeInfo.Length = diskHandle.Capacity()
	return nil
}

func (d *VadpDumper) Cleanup() {
	if d.diskHandle != nil {
		d.diskHandle.Close()
	}
	if d.writeHandle != nil {
		d.writeHandle.Close()
	}
	disklib.Exit()
}

func (d *VadpDumper) QueryAllocatedBlocks() (err error) {

	sectorSize  := int64(disklib.VIXDISKLIB_SECTOR_SIZE)
	blockSize   := uint64(2*1024) // 1MB block size
	blockCount  := uint64(d.diskInfo.Capacity) / blockSize
	maxChunkNum := uint64(disklib.VIXDISKLIB_MAX_CHUNK_NUMBER)
	log.Debugf("Current chunk info: chunk size: %v, chunk count: %v, Max count: %v", blockSize, blockSize, maxChunkNum)

	offset := uint64(0)
	for blockCount > 0 {
		onceCount := blockCount
		if blockCount > maxChunkNum {
			onceCount = maxChunkNum
		}

		startSector := disklib.VixDiskLibSectorType(offset)
		numSectors  := disklib.VixDiskLibSectorType(onceCount*blockSize)
		chunkSize   := disklib.VixDiskLibSectorType(blockSize)

		blockList, errVix := d.diskHandle.QueryAllocatedBlocks(startSector, numSectors, chunkSize)
		if errVix != nil {
			return fmt.Errorf("QueryAllocatedBlocks: %v", errVix)
		}

		for _, block := range blockList {
			//log.Printf("%+v\n", block)
			changed := ChangedArea{
				Start:  int64(block.Offset()) * sectorSize,
				Length: int64(block.Length()) * sectorSize,
			}
			d.ChangeInfo.ChangedArea = append(d.ChangeInfo.ChangedArea, changed)
		}

		blockCount -= onceCount
		offset += onceCount * blockSize
	}

	log.Infof("All ChangeInfo: \n%v\n", d.ChangeInfo)
	return nil
}


func NullTermToStrings(b []byte) (s []string) {
	// ref: Converting NULL terminated []byte to []string: https://groups.google.com/g/golang-nuts/c/E4Zhmc5xGus
	for {
		i := bytes.IndexByte(b, byte('\x00')) // why '\0' ???
		if i == -1 {
			break
		}
		s = append(s, string(b[0:i]))
		b = b[i+1:]
	}
	return
}

func (d *VadpDumper) SaveMetaData() (err error) {
	var requireLen uint

	// 获取需要的长度
	errVix := d.diskHandle.GetMetadataKeys( nil, 0, &requireLen)
	if errVix != nil && errVix.VixErrorCode() != disklib.VIX_E_BUFFER_TOOSMALL {
		return fmt.Errorf("GetMetadataKeys: %v", errVix.Error())
	}
	log.Infof("SaveMetaData: %v\n", requireLen)

	// 读取MetedataKeys
	bufLen := requireLen
	buf := make([]byte, bufLen)

	errVix = d.diskHandle.GetMetadataKeys(buf, bufLen, nil)
	if errVix != nil {
		return fmt.Errorf("GetMetadataKeys: %v", errVix.Error())
	}

	keys := NullTermToStrings(buf)
	log.Infof("MetadataKeysXXX: [%s]\n", keys)

	for _, key := range keys {
		errVix := d.diskHandle.ReadMetadata( key, nil, 0, &requireLen)
		if errVix != nil && errVix.VixErrorCode() != disklib.VIX_E_BUFFER_TOOSMALL {
			return fmt.Errorf("ReadMetadata: %v", errVix.Error())
		}
		log.Infof("Key: %v, RequireLen: %v", key, requireLen)

		bufLen = requireLen
		buf = make([]byte, bufLen)

		errVix = d.diskHandle.ReadMetadata( key, buf, bufLen, nil)
		if errVix != nil {
			return fmt.Errorf("ReadMetadata: %v", errVix.Error())
		}
		log.Infof("Key: %v, Buf: %v", key, string(buf[:]))
	}

	return nil
}

func (d *VadpDumper) OpenLocalDisk(diskName string) (err error) {
	if res := disklib.Init(d.Major, d.Minor, d.LibPath); res != nil {
		return fmt.Errorf("disklib.Init: %v", res)
	}

	vmxSpec    := ""
	servName   := ""
	thumbPrint := ""
	userName   := ""
	password   := ""
	identity   := ""
	path       := diskName
	flag       := uint32(0)
	readOnly   := true
	snapRef    := ""
	mode       := ""

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
		readOnly,
		snapRef,
		mode)

	conn, errVix := disklib.Connect(params)
	if errVix != nil {
		return fmt.Errorf("Connect: %v\n", errVix)
	}

	log.Infof("Connect success\n")
	d.lConnection = &conn

	// Open local disk
	dli, errVix := disklib.Open(conn, params)
	if errVix != nil {
		return fmt.Errorf("Open: %v", errVix)
	}
	log.Infof("Open success\n")

	info, errVix := disklib.GetInfo(dli)
	if errVix != nil {
		return fmt.Errorf("GetInfo: %v", errVix)
	}
	log.Infof("Local GetInfo: %+v\n", info)

	diskHandle := virtual_disks.NewDiskHandle(dli, conn, params, info)
	d.diskHandle = &diskHandle
	return nil
}

func (d *VadpDumper) CreateLocalDisk(diskName string, diskLen uint64) (err error) {
	if res := disklib.Init(d.Major, d.Minor, d.LibPath); res != nil {
		return fmt.Errorf("disklib.Init: %v", res)
	}
	//params := disklib.ConnectParams{}

	vmxSpec    := ""
	servName   := ""
	thumbPrint := ""
	userName   := ""
	password   := ""
	identity   := ""
	path       := diskName
	flag       := uint32(0)
	readOnly   := false
	snapRef    := ""
	mode       := ""

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
		readOnly,
		snapRef,
		mode)

	conn, errVix := disklib.Connect(params)
	if errVix != nil {
		return fmt.Errorf("Connect: %v\n", errVix)
	}

	log.Infof("Connect success\n")
	d.lConnection = &conn

	diskType    := disklib.VIXDISKLIB_DISK_VMFS_FLAT
	adapterType := disklib.VIXDISKLIB_ADAPTER_SCSI_LSILOGIC
	hwVersion   := uint16(7)
	capacity    := disklib.VixDiskLibSectorType(diskLen / disklib.VIXDISKLIB_SECTOR_SIZE)

	createParams := disklib.NewCreateParams(
		diskType,
		adapterType,
		hwVersion,
		capacity,
	)

	errVix = disklib.Create(conn, diskName, createParams, "")
	if errVix != nil {
		return fmt.Errorf("Create: %v\n", errVix)
	}
	log.Infof("Create success\n")

	// Open local disk
	dli, errVix := disklib.Open(conn, params)
	if errVix != nil {
		return fmt.Errorf("Open: %v", errVix)
	}
	log.Infof("Open success\n")

	info, errVix := disklib.GetInfo(dli)
	if errVix != nil {
		return fmt.Errorf("GetInfo: %v", errVix)
	}
	log.Infof("Local GetInfo: %+v\n", info)

	diskHandle := virtual_disks.NewDiskHandle(dli, conn, params, info)
	d.writeHandle = &diskHandle
	return nil
}

func (d *VadpDumper) ReadFromVmdk(buf []byte, offset int64) (n int, err error) {
	return d.diskHandle.ReadAt(buf, offset)
}

func (d *VadpDumper) WriteToVmdk(buf []byte, offset int64) (n int, err error) {
	return d.writeHandle.WriteAt(buf, offset)
}

func (d *VadpDumper) DumpCloneDisk(dc *DiskChangeInfo) (err error) {
	//sectorPer   := 1024
	//sectorSize  := disklib.VIXDISKLIB_SECTOR_SIZE

	// NOTE:
	// 每次读的大小为1MB, 也就是(2048个扇区, 每个扇区512Byte)
	sectorSize  := int64(disklib.VIXDISKLIB_SECTOR_SIZE * 1024 * 2)
	startOffset := dc.StartOffset

	//FIXME:
	// 待明确的地方， ReadAt的用法，没有限制长度的参数，读写数据是否有保证?
	block  := make([]byte, sectorSize)
	buffer := block
	for _, area := range dc.ChangedArea {
		log.Infof("CURRENT AREA: %+v", area)
		currOffset := startOffset + area.Start
		offsetLen  := area.Length

		maxOffset  := currOffset + offsetLen
		for currOffset < maxOffset {
			if maxOffset - currOffset < sectorSize {
				buffer = make([]byte, maxOffset-currOffset)
			} else {
				buffer = block
			}

			readLen, err := d.ReadFromVmdk(buffer, currOffset)
			if err != nil {
				return fmt.Errorf("ReadFromVmdk: %v", err)
			}
			writeLen, err := d.WriteToVmdk(buffer, currOffset)
			if err != nil {
				return fmt.Errorf("WriteToVmdk: %v", err)
			}

			currOffset += int64(readLen)
			if readLen != writeLen || int64(readLen) != sectorSize {
				log.Warnf("readLen: %v, writeLen: %v, sectorSize: %v", readLen, writeLen, sectorSize)
			}
		}
	}
	return nil
}

func (d *VadpDumper) DumpBackupDisk() (err error) {

	return nil
}

func (d *VadpDumper) DumpRestoreDisk(dc *DiskChangeInfo) (err error) {
	return d.DumpCloneDisk(dc)
}

