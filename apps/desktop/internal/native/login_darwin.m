#import <ServiceManagement/ServiceManagement.h>
#include <stdbool.h>

int openSurgeLoginStatus(void) {
    @autoreleasepool { return (int)SMAppService.mainAppService.status; }
}
bool setOpenSurgeLogin(bool enabled) {
    @autoreleasepool {
        NSError *error = nil;
        return enabled ? [SMAppService.mainAppService registerAndReturnError:&error]
                       : [SMAppService.mainAppService unregisterAndReturnError:&error];
    }
}
void openSurgeLoginSettings(void) { [SMAppService openSystemSettingsLoginItems]; }
bool openSurgeIsInstalledApp(void) {
    @autoreleasepool {
        NSBundle *bundle = NSBundle.mainBundle;
        return [bundle.bundleIdentifier isEqualToString:@"com.opensurge.menubar"] &&
            [bundle.bundleURL.URLByResolvingSymlinksInPath.path isEqualToString:@"/Applications/OpenSurge.app"];
    }
}
