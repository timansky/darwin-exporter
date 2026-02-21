// wifi_darwin.m — CoreWLAN event bridge for darwin-exporter.
// Compiled only when CGO_ENABLED=1 (via wifi_cgo.go #cgo pragma).

#import <CoreWLAN/CoreWLAN.h>
#import <Foundation/Foundation.h>
#include <CoreFoundation/CoreFoundation.h>
#include <string.h>

#include "wifi_darwin.h"

// Forward declaration of the Go-exported callback.
extern void goWiFiEventCallback(CWiFiData data);

// ---- Module-level globals ----
static CFRunLoopRef      gRunLoopRef = NULL;
static CWWiFiClient     *gClient     = nil;

// ---- WiFiEventDelegate ----
@interface WiFiEventDelegate : NSObject <CWEventDelegate>
@end

@implementation WiFiEventDelegate

- (void)linkQualityDidChangeForWiFiInterfaceWithName:(NSString *)interfaceName
                                               rssi:(NSInteger)rssi
                                        transmitRate:(double)transmitRate {
    CWiFiData data;
    pollCoreWLAN(&data);
    goWiFiEventCallback(data);
}

- (void)linkDidChangeForWiFiInterfaceWithName:(NSString *)interfaceName {
    CWiFiData data;
    pollCoreWLAN(&data);
    goWiFiEventCallback(data);
}

- (void)powerStateDidChangeForWiFiInterfaceWithName:(NSString *)interfaceName {
    CWiFiData data;
    pollCoreWLAN(&data);
    goWiFiEventCallback(data);
}

@end

// ---- pollCoreWLAN ----
void pollCoreWLAN(CWiFiData *out) {
    memset(out, 0, sizeof(CWiFiData));

    if (!gClient) {
        return;
    }
    CWInterface *iface = gClient.interface;
    if (!iface) {
        return;
    }
    if (iface.interfaceName) {
        const char *ifname = [iface.interfaceName UTF8String];
        if (ifname) {
            strncpy(out->interface_name, ifname, sizeof(out->interface_name) - 1);
        }
    }

    out->power_on = iface.powerOn ? 1 : 0;
    if (!iface.powerOn) {
        return;
    }

    out->connected = (iface.wlanChannel != nil) ? 1 : 0;
    out->rssi      = (int)iface.rssiValue;
    out->noise     = (int)iface.noiseMeasurement;
    out->tx_rate   = iface.transmitRate;
    out->tx_power  = (int)iface.transmitPower;
    out->phy_mode  = (int)iface.activePHYMode;

    CWChannel *ch = iface.wlanChannel;
    if (ch) {
        out->channel       = (int)ch.channelNumber;
        out->channel_width = (int)ch.channelWidth;
        out->channel_band  = (int)ch.channelBand;
    }

    if (iface.ssid) {
        const char *utf8 = [iface.ssid UTF8String];
        if (utf8) {
            strncpy(out->ssid, utf8, sizeof(out->ssid) - 1);
        }
    }

    // Map CWSecurity enum to a human-readable string.
    // CWSecurity does not require Location Services.
    switch (iface.security) {
        case kCWSecurityNone:
            strncpy(out->security, "none", sizeof(out->security) - 1);
            break;
        case kCWSecurityWEP:
            strncpy(out->security, "WEP", sizeof(out->security) - 1);
            break;
        case kCWSecurityWPAPersonal:
        case kCWSecurityWPAPersonalMixed:
            strncpy(out->security, "WPA", sizeof(out->security) - 1);
            break;
        case kCWSecurityWPA2Personal:
            strncpy(out->security, "WPA2", sizeof(out->security) - 1);
            break;
        case kCWSecurityWPA3Personal:
            strncpy(out->security, "WPA3", sizeof(out->security) - 1);
            break;
        case kCWSecurityWPA3Enterprise:
        case kCWSecurityWPA2Enterprise:
        case kCWSecurityWPAEnterprise:
        case kCWSecurityWPAEnterpriseMixed:
            strncpy(out->security, "WPA-Enterprise", sizeof(out->security) - 1);
            break;
        case kCWSecurityDynamicWEP:
            strncpy(out->security, "DynamicWEP", sizeof(out->security) - 1);
            break;
        default:
            strncpy(out->security, "unknown", sizeof(out->security) - 1);
            break;
    }
}

// ---- startCoreWLANMonitor ----
void startCoreWLANMonitor(void) {
    @autoreleasepool {
        gRunLoopRef = CFRunLoopGetCurrent();
        gClient     = [CWWiFiClient sharedWiFiClient];

        WiFiEventDelegate *delegate = [[WiFiEventDelegate alloc] init];
        gClient.delegate = delegate;

        NSError *err = nil;
        [gClient startMonitoringEventWithType:CWEventTypeLinkQualityDidChange error:&err];
        if (err) {
            NSLog(@"CoreWLAN: failed to monitor linkQualityDidChange: %@", err);
            err = nil;
        }
        [gClient startMonitoringEventWithType:CWEventTypeLinkDidChange error:&err];
        if (err) {
            NSLog(@"CoreWLAN: failed to monitor linkDidChange: %@", err);
            err = nil;
        }
        [gClient startMonitoringEventWithType:CWEventTypePowerDidChange error:&err];
        if (err) {
            NSLog(@"CoreWLAN: failed to monitor powerDidChange: %@", err);
        }

        // Eager initial poll so the first Prometheus scrape has data.
        CWiFiData data;
        pollCoreWLAN(&data);
        goWiFiEventCallback(data);

        // Block until stopCoreWLANMonitor() calls CFRunLoopStop().
        CFRunLoopRun();
    }
}

// ---- stopCoreWLANMonitor ----
void stopCoreWLANMonitor(void) {
    if (gClient) {
        [gClient stopMonitoringAllEventsAndReturnError:nil];
    }
    if (gRunLoopRef) {
        CFRunLoopStop(gRunLoopRef);
        gRunLoopRef = NULL;
    }
}
