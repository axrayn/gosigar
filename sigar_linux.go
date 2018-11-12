// Copyright (c) 2012 VMware, Inc.

package gosigar

import (
	"io/ioutil"
	"strconv"
	"strings"
	"syscall"
 
)

func init() {
	system.ticks = 100 // C.sysconf(C._SC_CLK_TCK)

	Procd = "/proc"

	getLinuxBootTime()
}

func getMountTableFileName() string {
	return "/etc/mtab"
}

func (self *Uptime) Get() error {
	sysinfo := syscall.Sysinfo_t{}

	if err := syscall.Sysinfo(&sysinfo); err != nil {
		return err
	}

	self.Length = float64(sysinfo.Uptime)

	return nil
}

func (self *FDUsage) Get() error {
	return readFile(Procd+"/sys/fs/file-nr", func(line string) bool {
		fields := strings.Fields(line)
		if len(fields) == 3 {
			self.Open, _ = strconv.ParseUint(fields[0], 10, 64)
			self.Unused, _ = strconv.ParseUint(fields[1], 10, 64)
			self.Max, _ = strconv.ParseUint(fields[2], 10, 64)
		}
		return false
	})
}

func (self *HugeTLBPages) Get() error {
	table, err := parseMeminfo()
	if err != nil {
		return err
	}

	self.Total, _ = table["HugePages_Total"]
	self.Free, _ = table["HugePages_Free"]
	self.Reserved, _ = table["HugePages_Rsvd"]
	self.Surplus, _ = table["HugePages_Surp"]
	self.DefaultSize, _ = table["Hugepagesize"]

	if totalSize, found := table["Hugetlb"]; found {
		self.TotalAllocatedSize = totalSize
	} else {
		// If Hugetlb is not present, or huge pages of different sizes
		// are used, this figure can be unaccurate.
		// TODO (jsoriano): Extract information from /sys/kernel/mm/hugepages too
		self.TotalAllocatedSize = (self.Total - self.Free + self.Reserved) * self.DefaultSize
	}

	return nil
}

func (self *ProcFDUsage) Get(pid int) error {
	err := readFile(procFileName(pid, "limits"), func(line string) bool {
		if strings.HasPrefix(line, "Max open files") {
			fields := strings.Fields(line)
			if len(fields) == 6 {
				self.SoftLimit, _ = strconv.ParseUint(fields[3], 10, 64)
				self.HardLimit, _ = strconv.ParseUint(fields[4], 10, 64)
			}
			return false
		}
		return true
	})
	if err != nil {
		return err
	}
	fds, err := ioutil.ReadDir(procFileName(pid, "fd"))
	if err != nil {
		return err
	}
	self.Open = uint64(len(fds))
	return nil
}

func parseCpuStat(self *Cpu, line string) error {
	fields := strings.Fields(line)

	self.User, _ = strtoull(fields[1])
	self.Nice, _ = strtoull(fields[2])
	self.Sys, _ = strtoull(fields[3])
	self.Idle, _ = strtoull(fields[4])
	self.Wait, _ = strtoull(fields[5])
	self.Irq, _ = strtoull(fields[6])
	self.SoftIrq, _ = strtoull(fields[7])
	self.Stolen, _ = strtoull(fields[8])

	return nil
}

func (self *Mem) Get() error {

	table, err := parseMeminfo()
	if err != nil {
		return err
	}

	self.Total, _ = table["MemTotal"]
	self.Free, _ = table["MemFree"]
	buffers, _ := table["Buffers"]
	cached, _ := table["Cached"]

	if available, ok := table["MemAvailable"]; ok {
		// MemAvailable is in /proc/meminfo (kernel 3.14+)
		self.ActualFree = available
	} else {
		self.ActualFree = self.Free + buffers + cached
	}

	self.Used = self.Total - self.Free
	self.ActualUsed = self.Total - self.ActualFree

	return nil
}

func (self *CpuInfoList) Get() error {
       // We'll need to read in the /proc/cpuinfo file but because it lists all the cpu details inline we'll need to keep a running 'tally' 
       capacity := len(self.List)
       if capacity == 0 {
	 capacity = 4
       }
       list := make([]CpuInfo, 0, capacity)
       var ci CpuInfo
       var hasProc bool

       err := readFile(Procd+"/cpuinfo", func(line string) bool {
	 fields := strings.SplitN(line, ":", 2)
	 if len(fields) == 2 {
	     key := strings.TrimSpace(fields[0])
	     value := strings.TrimSpace(fields[1])
	     switch key {
	         case "processor":
	             // Processor line that we use to set the procid and then make the CpuList object
		     if hasProc == false {
		       hasProc = true
		     } else {
		       list = append(list, ci)
		     }

	             ci.Processor, _ = strconv.ParseInt(value, 10, 8)
		   case "vendor_id":
		     ci.VendorID = value
		   case "cpu family":
		     ci.CPUFamily, _ = strconv.ParseInt(value, 10, 8)
		   case "model":
		     ci.Model, _ = strconv.ParseInt(value, 10, 8)
		   case "model name":
		     ci.ModelName = value
		   case "stepping":
		     ci.Stepping,_ = strconv.ParseInt(value, 10, 8)
		   case "microcode":
		     ci.Microcode = value
		   case "cpu MHz":
		     ci.CPUMHz, _ = strconv.ParseFloat(value, 64)
		   case "cache size":
		     if cs, err := strconv.ParseInt(strings.TrimSuffix(value, " KB"), 10, 16); err == nil {
		         ci.CacheSize = cs
		     } else {
		       // Shouldn't happen, but.......
		       ci.CacheSize = 0
		     }
		   case "physical id":
		     ci.PhysicalID, _ = strconv.ParseInt(value, 10, 8)
		   case "siblings":
		     ci.Siblings, _ = strconv.ParseInt(value, 10, 8)
		   case "core id":
		     ci.CoreID, _ = strconv.ParseInt(value, 10, 8)
		   case "cpu cores":
		     ci.CPUCores, _ = strconv.ParseInt(value, 10, 8)
		   case "apicid":
		     ci.Apicid, _ = strconv.ParseInt(value, 10, 8)
		   case "initial apicid":
		     ci.InitialApicid, _ = strconv.ParseInt(value, 10, 8)
		   case "fpu":
		     if value  == "yes" {
		       ci.Fpu = true
		     } else {
		       ci.Fpu = false
		     }
		   case "fpu_exception":
                     if value  == "yes" {
                       ci.FpuException = true
                     } else {
                       ci.FpuException = false
                     }
		   case "cpuid level":
		     ci.CPUIDLevel, _ = strconv.ParseInt(value, 10, 8)
		   case "wp":
                     if value  == "yes" {
                       ci.Wp = true
                     } else {
                       ci.Wp = false
                     }
		   case "flags":
		     ci.Flags = strings.Fields(value)
		   case "bugs":
		     ci.Bugs = strings.Fields(value)
		   case "bogomips":
		     ci.Bogomips, _ = strconv.ParseFloat(value, 64)
		   case "clflush size":
		     ci.ClFlushSize, _ = strconv.ParseInt(value, 10, 8)
		   case "cache_alignment":
		     ci.CacheAlignment, _ = strconv.ParseInt(value, 10, 8)
		   case "address sizes":
	             sizes := []int64{0,0}
	             bits := strings.Split(value, ", ")
	             if bp, err := strconv.ParseInt(strings.TrimSuffix(bits[0], " bits physical"), 10, 16); err == nil {
	                 sizes[0] = bp
	             }
	             if bv, err := strconv.ParseInt(strings.TrimSuffix(bits[1], " bits virtual"), 10, 16); err == nil {
	                 sizes[1] = bv
	             }
		     ci.AddressSizes = sizes
		   case "power management":
		     ci.PowerManagement = strings.Fields(value)
		   default:
		 }
            }
	    return true
        })
	// This catches the last processor's lines (if any)
	if hasProc == false {
            hasProc = true
        } else {
            list = append(list, ci)
        }
	self.List = list

	return err
}
