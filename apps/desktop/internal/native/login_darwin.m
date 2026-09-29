#import <ServiceManagement/ServiceManagement.h>
#include <stdbool.h>
#include <string.h>

const char *openSurgeLoginStatus(void) {
    @autoreleasepool {
        switch (SMAppService.mainAppService.status) {
            case SMAppServiceStatusNotRegistered: return "disabled";
            case SMAppServiceStatusEnabled: return "enabled";
            case SMAppServiceStatusRequiresApproval: return "approval";
            // This also occurs for a new or replaced ad-hoc-signed bundle.
            // An explicit register request can recover it.
            case SMAppServiceStatusNotFound: return "not_found";
            default: return "unavailable";
        }
    }
}
bool setOpenSurgeLogin(bool enabled, char **message) {
    @autoreleasepool {
        *message = NULL;
        NSError *error = nil;
        BOOL ok = enabled ? [SMAppService.mainAppService registerAndReturnError:&error]
                          : [SMAppService.mainAppService unregisterAndReturnError:&error];
        if (!ok && error) {
            NSString *detail = [NSString stringWithFormat:@"%@: %@ (%ld)",
                error.domain, error.localizedDescription, (long)error.code];
            *message = strdup(detail.UTF8String);
        }
        return ok;
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
