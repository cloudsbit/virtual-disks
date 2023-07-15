package dumper

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/cloudsbit/virtual-disks/v2/pkg/disklib"
	"github.com/cloudsbit/virtual-disks/v2/pkg/virtual_disks"
	log "github.com/sirupsen/logrus"
)

var (
	ErrConnParam  = errors.New("vddk: Invalid vsphere connect params")
	ErrDiskHandle = errors.New("vddk: Invalid disk handle")
)

type DumpMode int

const (
	DumpBlocks = iota
	DumpBackup
	DumpClone
	DumpResotre
)

type VddkVersion struct {
	Major   uint32
	Minor   uint32
	LibPath string
}

type VddkParams struct {
	Identity string
	ConnParams
	DiskParams
}

type VadpDumper struct {
	VddkParams
	DumpMode         DumpMode
	RemoteConnParams *disklib.ConnectParams

	remoteConnect  *disklib.VixDiskLibConnection
	remoteHandle   *disklib.VixDiskLibHandle
	remoteDiskInfo *disklib.VixDiskLibInfo

	readHandle  *virtual_disks.DiskConnectHandle
	writeHandle *virtual_disks.DiskConnectHandle

	LocalConnParams *disklib.ConnectParams
	localConnect    *disklib.VixDiskLibConnection
	localHandle     *disklib.VixDiskLibHandle

	ChangeInfo *DiskChangeInfo
}

func GetThumbPrintForServer(host string, port int) (string, error) {
	strPort := strconv.FormatInt(int64(port), 10)
	return disklib.GetThumbPrintForServer(host, strPort)
}

func getDiskLibFlag(mode DumpMode) uint32 {
	// Only for vsphere VM
	flag := 0
	if mode == DumpBlocks || mode == DumpBackup {
		flag |= disklib.VIXDISKLIB_FLAG_OPEN_READ_ONLY
	}
	return uint32(flag)
}

func isReadOnly(mode DumpMode) bool {
	// Only for vsphere VM
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
		VmMoRef:              moRef,
		VsphereHostName:      host,
		VsphereHostPort:      port,
		VsphereUsername:      name,
		VspherePassword:      password,
		VsphereThumbPrint:    thumbPrint,
		VsphereSnapshotMoRef: snapRef,
	}
	return params, nil
}

func NewVddkParams(conn ConnParams, disk DiskParams) (*VddkParams, error) {
	rand.Seed(time.Now().UnixNano())
	identity := fmt.Sprintf("%v_%v", "rsb_dumper_", rand.Intn(1000))

	params := &VddkParams{}
	params.Identity = identity
	params.ConnParams = conn
	params.DiskParams = disk

	return params, nil
}

func NewVadpDumper(vp VddkParams, dm DumpMode) (*VadpDumper, error) {
	//connParams  := new(disklib.ConnectParams)
	//connection  := new(disklib.VixDiskLibConnection)
	//diskInfo    := new(disklib.VixDiskLibInfo)
	//diskHandle  := new(virtual_disks.DiskConnectHandle)
	//changeInfo  := new(DiskChangeInfo)
	//progress    := new(DiskProgress)
	//lConnection := new(disklib.VixDiskLibConnection)
	//writeHandle := new(virtual_disks.DiskConnectHandle)

	dumper := &VadpDumper{}
	dumper.VddkParams = vp
	dumper.DumpMode = dm

	return dumper, nil
}

func (d *VadpDumper) SetRemoteConnParams(readOnly bool) {
	vmxSpec := d.VmMoRef
	servName := d.VsphereHostName
	thumbPrint := d.VsphereThumbPrint
	userName := d.VsphereUsername
	password := d.VspherePassword
	identity := d.Identity
	path := d.DiskPathRoot
	flag := getDiskLibFlag(d.DumpMode)
	snapRef := d.VsphereSnapshotMoRef
	transMode := disklib.NBD // FIXME

	// 连接到vsphere对应的vm及其disk的参数
	connParams := disklib.NewConnectParams(
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
		transMode)

	log.Infof("Remote Disk ConnectParams: %v", connParams)
	d.RemoteConnParams = &connParams
}

func (d *VadpDumper) SetLocalConnParams(diskName string, readOnly bool) {
	vmxSpec := ""
	servName := ""
	thumbPrint := ""
	userName := ""
	password := ""
	identity := ""
	path := diskName
	flag := uint32(0)
	snapRef := ""
	mode := ""

	connParams := disklib.NewConnectParams(
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

	log.Infof("Local Disk ConnectParams: %v", connParams)
	d.LocalConnParams = &connParams
}

// NOTE: VddkLibInit只能在主线程调用一次
// 参考链接：Multithreading Considerations:
// https://code.vmware.com/docs/4076/virtual-disk-development-kit-programming-guide/doc/vddkFunctions.6.13.html
func VddkLibInit(ver VddkVersion) error {
	//FIXME: Init函数里面的参数待增加优化...
	return disklib.Init(ver.Major, ver.Minor, ver.LibPath)
}

// NOTE: 去初始化，也只调用一次
func VddkLibDeInit() {
	disklib.Exit()
}

// NOTE: PrepareForAccess这里是针对整个vm的，而非单个disk, 正确的用法是创建快照之前调用该函数, 该函数不能用于ESXi host.
// 参考链接：vddk doc 7.0: Prepare For Access and End Access
func (d *VadpDumper) PrepareForAccess() error {
	if d.RemoteConnParams == nil {
		return ErrConnParam
	}
	params := *d.RemoteConnParams

	var errVix disklib.VddkError
	for i := 0; i < 10; i++ {
		errVix = disklib.PrepareForAccess(params)
		if errVix == nil {
			return nil
		}
		log.Warnf("PrepareForAccess: %v", errVix)

		disklib.EndAccess(params)
		time.Sleep(time.Duration(2) * time.Second)
	}

	return fmt.Errorf("PrepareForAccess error: %v\n", errVix)
}

// NOTE:
// 该函数可以用做程序崩溃时的清理
func (d *VadpDumper) EndAccess() error {
	if d.RemoteConnParams == nil {
		return ErrConnParam
	}
	params := *d.RemoteConnParams

	var errVix disklib.VddkError
	for i := 0; i < 30; i++ {
		errVix = disklib.EndAccess(params)
		if errVix == nil {
			d.libCleanup(params)
			return nil
		}
		log.Warnf("EndAccess: %v", errVix)
		time.Sleep(time.Duration(2) * time.Second)
	}
	return fmt.Errorf("EndAccess error: %v\n", errVix)
}

func (d *VadpDumper) libCleanup(params disklib.ConnectParams) {
	var numCleanUp, numRemaining uint32
	vErr := disklib.Cleanup(params, numCleanUp, numRemaining)
	if vErr != nil {
		log.Warnf(vErr.Error()+" with error code: %d", vErr.VixErrorCode())
	}
}

func (d *VadpDumper) Cleanup() error {
	if d.remoteHandle != nil {
		vErr := disklib.Close(*d.remoteHandle)
		if vErr != nil {
			log.Warnf(vErr.Error()+" with error code: %d", vErr.VixErrorCode())
		}
	}
	if d.remoteConnect != nil {
		vErr := disklib.Disconnect(*d.remoteConnect)
		if vErr != nil {
			log.Warnf(vErr.Error()+" with error code: %d", vErr.VixErrorCode())
		}
	}

	if d.localHandle != nil {
		vErr := disklib.Close(*d.localHandle)
		if vErr != nil {
			log.Warnf(vErr.Error()+" with error code: %d", vErr.VixErrorCode())
		}
	}
	if d.localConnect != nil {
		vErr := disklib.Disconnect(*d.localConnect)
		if vErr != nil {
			log.Warnf(vErr.Error()+" with error code: %d", vErr.VixErrorCode())
		}
	}

	return nil
}

func (d *VadpDumper) OpenRemoteDisk() (err error) {
	// NOTE:
	// 这里连接到vsphere关联的vm, 并open其相关的的disk

	//var numCleanUp, numRemaining uint32
	//disklib.Cleanup(*d.vsphereConnParams, numCleanUp, numRemaining)

	if d.RemoteConnParams == nil {
		return ErrConnParam
	}
	params := *d.RemoteConnParams

	conn, errVix := disklib.ConnectEx(params)
	if errVix != nil {
		return fmt.Errorf("disklib.ConnectEx: %v", errVix)
	}

	d.remoteConnect = &conn
	log.Infof("Connect to remote disk success\n")

	defer func() {
		if err != nil {
			disklib.Disconnect(conn)
		}
	}()

	dli, errVix := disklib.Open(conn, params)
	if errVix != nil {
		return fmt.Errorf("disklib.Open: %v\n", errVix)
	}

	d.remoteHandle = &dli
	log.Infof("Open remote disk success\n")

	defer func() {
		if err != nil {
			disklib.Close(dli)
		}
	}()

	diskInfo, errVix := disklib.GetInfo(dli)
	if errVix != nil {
		return fmt.Errorf("disklib.GetInfo: %v", errVix)
	}

	d.remoteDiskInfo = &diskInfo
	log.Infof("Get remote disk info: %+v\n", diskInfo)

	diskHandle := virtual_disks.NewDiskHandle(dli, conn, params, diskInfo)
	if d.DumpMode == DumpResotre {
		d.writeHandle = &diskHandle
	} else {
		d.readHandle = &diskHandle
	}
	return nil
}

func (d *VadpDumper) QueryAllocatedBlocks() (err error) {
	// 初始化ChangeInfo
	d.ChangeInfo = &DiskChangeInfo{
		StartOffset: 0,
		Length:      d.readHandle.Capacity(),
	}

	sectorSize := int64(disklib.VIXDISKLIB_SECTOR_SIZE)
	blockSize := uint64(2 * 1024) // 1MB block size
	blockCount := uint64(d.remoteDiskInfo.Capacity) / blockSize
	maxChunkNum := uint64(disklib.VIXDISKLIB_MAX_CHUNK_NUMBER)
	log.Debugf("Current chunk info: chunk size: %v, chunk count: %v, Max count: %v", blockSize, blockSize, maxChunkNum)

	offset := uint64(0)
	for blockCount > 0 {
		onceCount := blockCount
		if blockCount > maxChunkNum {
			onceCount = maxChunkNum
		}

		startSector := disklib.VixDiskLibSectorType(offset)
		numSectors := disklib.VixDiskLibSectorType(onceCount * blockSize)
		chunkSize := disklib.VixDiskLibSectorType(blockSize)

		blockList, errVix := d.readHandle.QueryAllocatedBlocks(startSector, numSectors, chunkSize)
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
	if d.readHandle == nil || d.writeHandle == nil {
		return ErrDiskHandle
	}

	var requireLen uint

	// 获取需要的长度
	errVix := d.readHandle.GetMetadataKeys(nil, 0, &requireLen)
	if errVix != nil && errVix.VixErrorCode() != disklib.VIX_E_BUFFER_TOOSMALL {
		return fmt.Errorf("GetMetadataKeys: %v", errVix.Error())
	}
	log.Infof("SaveMetaData: %v\n", requireLen)

	// 读取MetedataKeys
	bufLen := requireLen
	buf := make([]byte, bufLen)

	errVix = d.readHandle.GetMetadataKeys(buf, bufLen, nil)
	if errVix != nil {
		return fmt.Errorf("GetMetadataKeys: %v", errVix.Error())
	}

	keys := NullTermToStrings(buf)
	log.Infof("MetadataKeysXXX: [%s]\n", keys)

	for _, key := range keys {
		if len(strings.TrimSpace(key)) == 0 {
			continue
		}

		errVix := d.readHandle.ReadMetadata(key, nil, 0, &requireLen)
		if errVix != nil && errVix.VixErrorCode() != disklib.VIX_E_BUFFER_TOOSMALL {
			return fmt.Errorf("ReadMetadata: %v", errVix.Error())
		}
		log.Infof("Key: %v, RequireLen: %v", key, requireLen)

		bufLen = requireLen
		buf = make([]byte, bufLen)

		errVix = d.readHandle.ReadMetadata(key, buf, bufLen, nil)
		if errVix != nil {
			return fmt.Errorf("ReadMetadata: %v", errVix.Error())
		}
		log.Infof("Key: %v, Buf: %v", key, string(buf[:]))

		errVix = d.writeHandle.WriteMetadata(key, buf)
		if errVix != nil {
			return fmt.Errorf("WriteMetadata: %v", errVix.Error())
		}
	}

	return nil
}

func (d *VadpDumper) ReadLocalDisk() (err error) {
	if d.LocalConnParams == nil {
		return ErrConnParam
	}
	params := *d.LocalConnParams

	conn, errVix := disklib.Connect(params)
	if errVix != nil {
		return fmt.Errorf("disklib.Connect: %v\n", errVix)
	}

	d.localConnect = &conn
	log.Infof("Connect to local success\n")

	// Open local disk
	dli, errVix := disklib.Open(conn, params)
	if errVix != nil {
		return fmt.Errorf("disklib.Open: %v\n", errVix)
	}

	d.localHandle = &dli
	log.Infof("Open local disk success\n")

	info, errVix := disklib.GetInfo(dli)
	if errVix != nil {
		return fmt.Errorf("disklib.GetInfo: %v", errVix)
	}
	log.Infof("Get local disk GetInfo: %+v\n", info)

	diskHandle := virtual_disks.NewDiskHandle(dli, conn, params, info)
	d.readHandle = &diskHandle
	return nil
}

func (d *VadpDumper) CreateLocalDisk(diskName string, diskLen uint64) (err error) {
	if d.LocalConnParams == nil {
		return ErrConnParam
	}
	params := *d.LocalConnParams

	conn, errVix := disklib.Connect(params)
	if errVix != nil {
		return fmt.Errorf("disklib.Connect: %v\n", errVix)
	}

	d.localConnect = &conn
	log.Infof("Connect to local success\n")

	diskType := disklib.VIXDISKLIB_DISK_VMFS_FLAT
	adapterType := disklib.VIXDISKLIB_ADAPTER_SCSI_LSILOGIC
	hwVersion := uint16(7)
	capacity := disklib.VixDiskLibSectorType(diskLen / disklib.VIXDISKLIB_SECTOR_SIZE)

	createParams := disklib.NewCreateParams(
		diskType,
		adapterType,
		hwVersion,
		capacity,
	)

	// create local disk
	errVix = disklib.Create(conn, diskName, createParams, "")
	if errVix != nil {
		return fmt.Errorf("disklib.Create: %v\n", errVix)
	}
	log.Infof("Create local disk success\n")

	// Open local disk
	dli, errVix := disklib.Open(conn, params)
	if errVix != nil {
		return fmt.Errorf("disklib.Open: %v", errVix)
	}

	d.localHandle = &dli
	log.Infof("Open local disk success\n")

	info, errVix := disklib.GetInfo(dli)
	if errVix != nil {
		return fmt.Errorf("disklib.GetInfo: %v", errVix)
	}
	log.Infof("Get local disk GetInfo: %+v\n", info)

	diskHandle := virtual_disks.NewDiskHandle(dli, conn, params, info)
	d.writeHandle = &diskHandle
	return nil
}

func (d *VadpDumper) ReadFromVmdk(buf []byte, offset int64) (n int, err error) {
	if d.readHandle == nil {
		return 0, ErrDiskHandle
	}
	return d.readHandle.ReadAt(buf, offset)
}

func (d *VadpDumper) WriteToVmdk(buf []byte, offset int64) (n int, err error) {
	if d.writeHandle == nil {
		return 0, ErrDiskHandle
	}
	return d.writeHandle.WriteAt(buf, offset)
}

func (d *VadpDumper) DumpCloneDisk(dc *DiskChangeInfo) (err error) {
	//sectorPer   := 1024
	//sectorSize  := disklib.VIXDISKLIB_SECTOR_SIZE

	// NOTE:
	// 每次读的大小为1MB, 也就是(2048个扇区, 每个扇区512Byte)
	sectorSize := int64(disklib.VIXDISKLIB_SECTOR_SIZE * 1024 * 2)
	startOffset := dc.StartOffset

	//FIXME:
	// 待明确的地方， ReadAt的用法，没有限制长度的参数，读写数据是否有保证?
	block := make([]byte, sectorSize)
	buffer := block
	for _, area := range dc.ChangedArea {
		log.Infof("CURRENT AREA: %+v", area)

		currOffset := startOffset + area.Start
		offsetLen := area.Length

		maxOffset := currOffset + offsetLen
		for currOffset < maxOffset {
			if maxOffset-currOffset < sectorSize {
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
