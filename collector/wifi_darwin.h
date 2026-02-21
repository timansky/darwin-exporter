#ifndef WIFI_DARWIN_H
#define WIFI_DARWIN_H

// CWiFiData holds WiFi state captured from CoreWLAN API.
typedef struct {
    int    connected;      // 0 or 1
    int    power_on;       // 0 or 1
    int    rssi;           // dBm
    int    noise;          // dBm
    double tx_rate;        // Mbps
    int    tx_power;       // mW
    int    channel;        // channel number
    int    channel_width;  // CWChannelWidth enum: 0=20MHz,1=40MHz,2=80MHz,3=160MHz
    int    channel_band;   // CWChannelBand enum: 0=unknown,1=2GHz,2=5GHz,3=6GHz
    int    phy_mode;       // CWPHYMode enum: 0-6
    char   interface_name[32]; // interface name (e.g. en0, awdl0)
    char   ssid[256];      // SSID (null-terminated UTF-8)
    char   security[64];   // security mode string (null-terminated)
} CWiFiData;

// startCoreWLANMonitor initialises CoreWLAN, polls initial state, calls
// goWiFiEventCallback, registers event monitoring, then runs CFRunLoopRun().
// This function blocks until stopCoreWLANMonitor() is called.
void startCoreWLANMonitor(void);

// stopCoreWLANMonitor unregisters events and stops the RunLoop started by
// startCoreWLANMonitor().
void stopCoreWLANMonitor(void);

// pollCoreWLAN fills *out with the current state of the default WiFi interface.
void pollCoreWLAN(CWiFiData *out);

#endif /* WIFI_DARWIN_H */
