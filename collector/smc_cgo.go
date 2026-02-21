//go:build darwin && cgo

package collector

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <arpa/inet.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

typedef struct {
	char major;
	char minor;
	char build;
	char reserved;
	uint16_t release;
} smc_vers_t;

typedef struct {
	uint16_t version;
	uint16_t length;
	uint32_t cpuPLimit;
	uint32_t gpuPLimit;
	uint32_t memPLimit;
} smc_plimit_t;

typedef struct {
	uint32_t dataSize;
	uint32_t dataType;
	uint8_t dataAttributes;
} smc_key_info_t;

typedef struct {
	uint32_t key;
	smc_vers_t vers;
	smc_plimit_t pLimitData;
	smc_key_info_t keyInfo;
	uint8_t result;
	uint8_t status;
	uint8_t data8;
	uint32_t data32;
	uint8_t bytes[32];
} smc_key_data_t;

static inline uint32_t smc_fourcc(const char key[4]) {
	return ((uint32_t)(uint8_t)key[0] << 24) |
	       ((uint32_t)(uint8_t)key[1] << 16) |
	       ((uint32_t)(uint8_t)key[2] << 8) |
	       ((uint32_t)(uint8_t)key[3]);
}

static inline void smc_key_to_chars(uint32_t key, char out[5]) {
	out[0] = (char)(key >> 24);
	out[1] = (char)(key >> 16);
	out[2] = (char)(key >> 8);
	out[3] = (char)(key);
	out[4] = '\0';
}

static kern_return_t smc_call(io_connect_t conn, smc_key_data_t *in, smc_key_data_t *out) {
	size_t inSize = sizeof(*in);
	size_t outSize = sizeof(*out);
	return IOConnectCallStructMethod(conn, 2, in, inSize, out, &outSize);
}

static int smc_open(io_connect_t *connOut) {
	if (!connOut) {
		return 0;
	}

	CFMutableDictionaryRef matching = IOServiceMatching("AppleSMC");
	if (!matching) {
		return 0;
	}

	io_service_t service = IOServiceGetMatchingService(kIOMainPortDefault, matching);
	if (!service) {
		return 0;
	}

	io_connect_t conn = MACH_PORT_NULL;
	kern_return_t kr = IOServiceOpen(service, mach_task_self(), 0, &conn);
	IOObjectRelease(service);
	if (kr != KERN_SUCCESS || conn == MACH_PORT_NULL) {
		return 0;
	}

	*connOut = conn;
	return 1;
}

static void smc_close(io_connect_t conn) {
	if (conn != MACH_PORT_NULL) {
		IOServiceClose(conn);
	}
}

static int smc_read_key_info(io_connect_t conn, const char key[4], smc_key_info_t *keyInfo) {
	if (!keyInfo) {
		return 0;
	}

	smc_key_data_t in;
	smc_key_data_t out;
	memset(&in, 0, sizeof(in));
	memset(&out, 0, sizeof(out));

	in.key = smc_fourcc(key);
	in.data8 = 9; // read key info

	if (smc_call(conn, &in, &out) != KERN_SUCCESS || out.result != 0 || out.keyInfo.dataSize == 0) {
		return 0;
	}

	*keyInfo = out.keyInfo;
	return 1;
}

static int smc_read_key_value(io_connect_t conn, const char key[4], const smc_key_info_t *keyInfo, smc_key_data_t *outData) {
	if (!keyInfo || !outData) {
		return 0;
	}

	smc_key_data_t in;
	memset(&in, 0, sizeof(in));
	memset(outData, 0, sizeof(*outData));

	in.key = smc_fourcc(key);
	in.keyInfo.dataSize = keyInfo->dataSize;
	in.data8 = 5; // read bytes

	if (smc_call(conn, &in, outData) != KERN_SUCCESS || outData->result != 0) {
		return 0;
	}

	return 1;
}

static int smc_decode_temperature(const smc_key_info_t *keyInfo, const smc_key_data_t *data, double *valueOut) {
	if (!keyInfo || !data || !valueOut) {
		return 0;
	}

	char type[5];
	smc_key_to_chars(keyInfo->dataType, type);

	if (keyInfo->dataSize == 4 && strcmp(type, "flt ") == 0) {
		float f = 0.0f;
		memcpy(&f, data->bytes, sizeof(f));
		*valueOut = (double)f;
		return 1;
	}

	if (keyInfo->dataSize == 2 && strcmp(type, "sp78") == 0) {
		uint16_t raw = 0;
		memcpy(&raw, data->bytes, sizeof(raw));
		*valueOut = (double)((int16_t)ntohs(raw)) / 256.0;
		return 1;
	}

	if (keyInfo->dataSize == 2 && strcmp(type, "fpe2") == 0) {
		uint16_t raw = 0;
		memcpy(&raw, data->bytes, sizeof(raw));
		*valueOut = (double)ntohs(raw) / 4.0;
		return 1;
	}

	if (keyInfo->dataSize == 1 && strcmp(type, "ui8 ") == 0) {
		*valueOut = (double)data->bytes[0];
		return 1;
	}

	return 0;
}

static int smc_read_temperature(io_connect_t conn, const char key[4], double *valueOut) {
	smc_key_info_t keyInfo;
	if (!smc_read_key_info(conn, key, &keyInfo)) {
		return 0;
	}

	smc_key_data_t data;
	if (!smc_read_key_value(conn, key, &keyInfo, &data)) {
		return 0;
	}

	return smc_decode_temperature(&keyInfo, &data, valueOut);
}

static int smc_read_key_count(io_connect_t conn, uint32_t *countOut) {
	if (!countOut) {
		return 0;
	}

	const char key[4] = {'#', 'K', 'E', 'Y'};
	smc_key_info_t keyInfo;
	if (!smc_read_key_info(conn, key, &keyInfo)) {
		return 0;
	}

	smc_key_data_t data;
	if (!smc_read_key_value(conn, key, &keyInfo, &data)) {
		return 0;
	}

	if (keyInfo.dataSize < 4) {
		return 0;
	}

	uint32_t count = ((uint32_t)data.bytes[0] << 24) |
	                 ((uint32_t)data.bytes[1] << 16) |
	                 ((uint32_t)data.bytes[2] << 8) |
	                 ((uint32_t)data.bytes[3]);
	*countOut = count;
	return 1;
}

static int smc_read_key_at(io_connect_t conn, uint32_t index, char outKey[5]) {
	if (!outKey) {
		return 0;
	}

	smc_key_data_t in;
	smc_key_data_t out;
	memset(&in, 0, sizeof(in));
	memset(&out, 0, sizeof(out));

	in.data8 = 8; // read key by index
	in.data32 = index;

	if (smc_call(conn, &in, &out) != KERN_SUCCESS || out.key == 0) {
		return 0;
	}

	smc_key_to_chars(out.key, outKey);
	return 1;
}
*/
import "C"

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unsafe"
)

// Reference:
// SMC request/response structs and temperature type decoding follow public
// reverse-engineering conventions from:
// - https://github.com/lavoiesl/osx-cpu-temp
// - https://github.com/sveinbjornt/SMCKit
var (
	smcCPUDiscoverKeyRe  = regexp.MustCompile(`^Tp[0-9A-Za-z]{2}$`)
	smcGPUDiscoverKeyRe  = regexp.MustCompile(`^T[gG][0-9A-Za-z]{2}$`)
	smcDiskDiscoverKeyRe = regexp.MustCompile(`^T[HN][0-9][A-Za-z]$`)
)

func discoverSMCKeySets() ([]string, []string, []string, error) {
	conn, err := openSMCConn()
	if err != nil {
		return nil, nil, nil, err
	}
	defer closeSMCConn(conn)

	keys, err := listSMCKeys(conn)
	if err != nil {
		return nil, nil, nil, err
	}

	cpuKeys := make([]string, 0, 96)
	gpuKeys := make([]string, 0, 16)
	diskKeys := make([]string, 0, 16)

	for _, key := range keys {
		switch {
		case smcCPUDiscoverKeyRe.MatchString(key):
			cpuKeys = append(cpuKeys, key)
		case smcGPUDiscoverKeyRe.MatchString(key):
			gpuKeys = append(gpuKeys, key)
		case smcDiskDiscoverKeyRe.MatchString(key):
			diskKeys = append(diskKeys, key)
		}
	}

	slices.Sort(cpuKeys)
	cpuKeys = slices.Compact(cpuKeys)

	slices.Sort(gpuKeys)
	gpuKeys = slices.Compact(gpuKeys)

	slices.Sort(diskKeys)
	diskKeys = slices.Compact(diskKeys)

	if len(cpuKeys) == 0 && len(gpuKeys) == 0 && len(diskKeys) == 0 {
		return nil, nil, nil, fmt.Errorf("no SMC cpu/gpu/disk keys discovered")
	}

	return cpuKeys, gpuKeys, diskKeys, nil
}

func readSMCKeyValues(keys []string) (map[string]float64, error) {
	conn, err := openSMCConn()
	if err != nil {
		return nil, err
	}
	defer closeSMCConn(conn)

	out := make(map[string]float64, len(keys))
	for _, key := range keys {
		if len(key) != 4 {
			continue
		}
		val, ok := readSMCKey(conn, key)
		if ok {
			out[key] = val
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no readable SMC values")
	}
	return out, nil
}

func openSMCConn() (C.io_connect_t, error) {
	var conn C.io_connect_t
	if C.smc_open(&conn) == 0 {
		return 0, fmt.Errorf("opening AppleSMC connection failed")
	}
	return conn, nil
}

func closeSMCConn(conn C.io_connect_t) {
	C.smc_close(conn)
}

func listSMCKeys(conn C.io_connect_t) ([]string, error) {
	var count C.uint32_t
	if C.smc_read_key_count(conn, &count) == 0 || count == 0 {
		return nil, fmt.Errorf("reading SMC key count failed")
	}

	keys := make([]string, 0, int(count))
	for i := C.uint32_t(0); i < count; i++ {
		var raw [5]C.char
		if C.smc_read_key_at(conn, i, &raw[0]) == 0 {
			continue
		}

		key := C.GoStringN(&raw[0], 4)
		if !isPrintableASCII(key) {
			continue
		}
		keys = append(keys, key)
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("SMC key list is empty")
	}
	return keys, nil
}

func readSMCKey(conn C.io_connect_t, key string) (float64, bool) {
	ckey := C.CString(key)
	defer C.free(unsafe.Pointer(ckey))

	var val C.double
	if C.smc_read_temperature(conn, ckey, &val) == 0 {
		return 0, false
	}
	return float64(val), true
}

func isPrintableASCII(key string) bool {
	if len(key) != 4 {
		return false
	}
	for _, r := range key {
		if r < 0x20 || r > 0x7e {
			return false
		}
	}
	return !strings.ContainsRune(key, 0)
}
