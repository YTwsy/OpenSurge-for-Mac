#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <objc/runtime.h>
#include <math.h>

static const char guardKey;
static BOOL smokeActions(void) { return [NSProcessInfo.processInfo.arguments containsObject:@"--smoke-actions"]; }

bool openSurgeSystemUsesEnglish(void) { return ![NSLocale.preferredLanguages.firstObject hasPrefix:@"zh"]; }

static BOOL isInternal(NSURL *url) {
    return [url.scheme.lowercaseString isEqualToString:@"wails"] &&
        [url.host.lowercaseString isEqualToString:@"localhost"] &&
        !url.user && !url.password && !url.port;
}

static BOOL isExternal(NSURL *url) {
    NSString *scheme = url.scheme.lowercaseString;
    return ([scheme isEqualToString:@"https"] || [scheme isEqualToString:@"http"]) &&
        url.host.length > 0 && !url.user && !url.password;
}

bool openSurgeExternalURLAllowed(const char *value) {
    return isExternal([NSURL URLWithString:[NSString stringWithUTF8String:value]]);
}

static WKWebView *findWebView(NSView *view) {
    if ([view isKindOfClass:WKWebView.class]) return (WKWebView *)view;
    for (NSView *child in view.subviews) {
        WKWebView *found = findWebView(child);
        if (found) return found;
    }
    return nil;
}

// Forward Wails' load, renderer-recovery and file-picker delegates. Keeping the
// original delegates preserves the framework's lifecycle without a Wails fork.
@interface OpenSurgeWebViewGuard : NSObject <WKNavigationDelegate, WKUIDelegate, WKScriptMessageHandler>
@property (weak) id<WKNavigationDelegate> navigation;
@property (weak) id<WKUIDelegate> ui;
@property (weak) WKWebView *webView;
@property BOOL english;
@property BOOL tray;
- (void)publishAppearance:(NSNotification *)notification;
- (void)publishTraySizing;
@end

@implementation OpenSurgeWebViewGuard
- (BOOL)respondsToSelector:(SEL)selector {
    return [super respondsToSelector:selector] || [self.navigation respondsToSelector:selector] || [self.ui respondsToSelector:selector];
}
- (id)forwardingTargetForSelector:(SEL)selector {
    if ([self.navigation respondsToSelector:selector]) return self.navigation;
    if ([self.ui respondsToSelector:selector]) return self.ui;
    return [super forwardingTargetForSelector:selector];
}
- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)action decisionHandler:(void (^)(WKNavigationActionPolicy))decision {
    NSURL *url = action.request.URL;
    BOOL localPage = isInternal(url) && ![url.path hasPrefix:@"/api/"] && ![url.path hasPrefix:@"/desktop/"];
    if (localPage && action.targetFrame.isMainFrame) {
        decision(WKNavigationActionPolicyAllow);
        return;
    }
    // Only a deliberate link activation can open the system browser. Redirects,
    // script navigation, subframes and file/custom schemes never leave the host.
    if (action.navigationType == WKNavigationTypeLinkActivated && action.sourceFrame.isMainFrame && isInternal(action.sourceFrame.request.URL) && isExternal(url)) {
        [NSWorkspace.sharedWorkspace openURL:url];
    }
    decision(WKNavigationActionPolicyCancel);
}
- (WKWebView *)webView:(WKWebView *)webView createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration forNavigationAction:(WKNavigationAction *)action windowFeatures:(WKWindowFeatures *)features {
    return nil;
}
- (void)publishAppearance:(NSNotification *)notification {
    if (!self.tray) return;
    WKWebView *webView = self.webView;
    [webView.effectiveAppearance performAsCurrentDrawingAppearance:^{
        NSDictionary<NSString *, NSColor *> *colours = @{
            @"--tray-label": NSColor.labelColor, @"--tray-secondary": NSColor.secondaryLabelColor,
            @"--tray-separator": NSColor.separatorColor, @"--tray-control": NSColor.controlColor,
            @"--tray-control-text": NSColor.controlTextColor, @"--tray-disabled": NSColor.disabledControlTextColor,
            @"--tray-accent": NSColor.controlAccentColor, @"--tray-accent-text": NSColor.alternateSelectedControlTextColor,
            @"--tray-download": NSColor.systemBlueColor, @"--tray-upload": NSColor.systemOrangeColor,
            @"--tray-solid": NSColor.windowBackgroundColor
        };
        NSMutableDictionary *values = [NSMutableDictionary dictionary];
        for (NSString *key in colours) {
            NSColor *colour = [colours[key] colorUsingColorSpace:NSColorSpace.sRGBColorSpace];
            if (colour) values[key] = [NSString stringWithFormat:@"rgba(%.0f,%.0f,%.0f,%.3f)", colour.redComponent * 255, colour.greenComponent * 255, colour.blueComponent * 255, colour.alphaComponent];
        }
        NSData *data = [NSJSONSerialization dataWithJSONObject:values options:0 error:nil];
        NSString *json = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
        NSString *script = [NSString stringWithFormat:@"Object.entries(%@).forEach(([key,value])=>document.documentElement.style.setProperty(key,value));", json];
        [webView evaluateJavaScript:script completionHandler:nil];
    }];
}
- (void)publishVisibility:(NSNotification *)notification {
    NSWindow *window = self.webView.window;
    BOOL visible = window.isVisible && !window.isMiniaturized && (window.occlusionState & NSWindowOcclusionStateVisible);
    NSString *script = [NSString stringWithFormat:@"window.__opensurgeWindowVisible=%@;document.dispatchEvent(new Event('visibilitychange'));", visible ? @"true" : @"false"];
    [self.webView evaluateJavaScript:script completionHandler:nil];
    if (visible) [self publishTraySizing];
}
- (void)publishTraySizing {
    if (!self.tray) return;
    NSWindow *window = self.webView.window;
    CGFloat available = MAX(240, NSHeight((window.screen ?: NSScreen.mainScreen).visibleFrame) - 16);
    NSString *script = [NSString stringWithFormat:
        @"document.documentElement.style.setProperty('--tray-available-height','%.0fpx');"
         "if(!window.__opensurgeTraySizing){window.__opensurgeTraySizing=true;"
         "const attach=()=>{const root=document.querySelector('.tray-app');if(!root)return false;"
         "let previous=0;new ResizeObserver(()=>{const height=Math.ceil(root.getBoundingClientRect().height);"
         "if(height!==previous){previous=height;window.webkit.messageHandlers.opensurgeTraySize.postMessage(height);}}).observe(root);return true;};"
         "if(!attach()){const pending=new MutationObserver(()=>{if(attach())pending.disconnect();});pending.observe(document.body,{childList:true,subtree:true});}}", available];
    [self.webView evaluateJavaScript:script completionHandler:nil];
}
- (void)userContentController:(WKUserContentController *)controller didReceiveScriptMessage:(WKScriptMessage *)message {
    if (!self.tray || ![message.name isEqualToString:@"opensurgeTraySize"] || !message.frameInfo.isMainFrame ||
        !isInternal(message.frameInfo.request.URL) || ![message.body isKindOfClass:NSNumber.class]) return;
    CGFloat height = [message.body doubleValue];
    if (!isfinite(height) || height < 100 || height > 2000) return;
    NSWindow *window = self.webView.window;
    NSRect available = (window.screen ?: NSScreen.mainScreen).visibleFrame;
    NSRect frame = window.frame;
    CGFloat top = MIN(NSMaxY(frame), NSMaxY(available) - 8);
    frame.size.height = MIN(ceil(height), NSHeight(available) - 16);
    frame.origin.y = MAX(NSMinY(available) + 8, top - frame.size.height);
    if (NSEqualRects(frame, window.frame)) return;
    // The disclosure animates its intrinsic height in CSS. Follow each measured
    // frame directly, preserving the menu-bar edge, instead of starting competing
    // native animations or scrolling the whole panel inside a fixed window.
    [window setFrame:frame display:YES];
    if (smokeActions()) NSLog(@"OpenSurge tray geometry: content=%.0f window=%.0f top=%.0f", height, NSHeight(window.frame), NSMaxY(window.frame));
}
- (void)webView:(WKWebView *)webView didFinishNavigation:(WKNavigation *)navigation {
    if ([self.navigation respondsToSelector:_cmd]) [self.navigation webView:webView didFinishNavigation:navigation];
    [self publishVisibility:nil];
    [self publishAppearance:nil];
    [self publishTraySizing];
}
- (NSAlert *)alert:(NSString *)message {
    NSAlert *alert = [NSAlert new];
    alert.messageText = @"OpenSurge";
    alert.informativeText = message;
    return alert;
}
- (void)webView:(WKWebView *)webView runJavaScriptAlertPanelWithMessage:(NSString *)message initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(void))completion {
    if (!frame.isMainFrame || !isInternal(frame.request.URL)) { completion(); return; }
    NSAlert *alert = [self alert:message];
    [alert addButtonWithTitle:self.english ? @"OK" : @"好"];
    [alert beginSheetModalForWindow:webView.window completionHandler:^(NSModalResponse result) { completion(); }];
}
- (void)webView:(WKWebView *)webView runJavaScriptConfirmPanelWithMessage:(NSString *)message initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(BOOL))completion {
    if (!frame.isMainFrame || !isInternal(frame.request.URL)) { completion(NO); return; }
    NSAlert *alert = [self alert:message];
    [alert addButtonWithTitle:self.english ? @"Continue" : @"继续"];
    [alert addButtonWithTitle:self.english ? @"Cancel" : @"取消"];
    [alert beginSheetModalForWindow:webView.window completionHandler:^(NSModalResponse result) { completion(result == NSAlertFirstButtonReturn); }];
}
- (void)dealloc { [NSNotificationCenter.defaultCenter removeObserver:self]; }
@end

void configureOpenSurgeWindow(void *pointer, bool rememberFrame) {
    NSWindow *window = (__bridge NSWindow *)pointer;
    WKWebView *webView = findWebView(window.contentView);
    if (!webView || objc_getAssociatedObject(webView, &guardKey)) return;
    OpenSurgeWebViewGuard *guard = [OpenSurgeWebViewGuard new];
    guard.webView = webView;
    guard.tray = !rememberFrame;
    if (guard.tray) {
        // Wails owns the effect view and transparent WKWebView. Use the AppKit
        // popover material rather than a fixed web colour or Liquid Glass.
        window.opaque = NO;
        window.backgroundColor = NSColor.clearColor;
        [webView.configuration.userContentController addScriptMessageHandler:guard name:@"opensurgeTraySize"];
        for (NSView *child in window.contentView.subviews) {
            if ([child isKindOfClass:NSVisualEffectView.class]) {
                NSVisualEffectView *effect = (NSVisualEffectView *)child;
                effect.material = NSVisualEffectMaterialPopover;
                effect.blendingMode = NSVisualEffectBlendingModeBehindWindow;
            }
        }
        [NSNotificationCenter.defaultCenter addObserver:guard selector:@selector(publishAppearance:) name:NSSystemColorsDidChangeNotification object:nil];
    }
    guard.navigation = webView.navigationDelegate;
    guard.ui = webView.UIDelegate;
    guard.english = ![NSLocale.preferredLanguages.firstObject hasPrefix:@"zh"];
    objc_setAssociatedObject(webView, &guardKey, guard, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
    webView.navigationDelegate = guard;
    webView.UIDelegate = guard;
    for (NSNotificationName name in @[NSWindowDidChangeOcclusionStateNotification, NSWindowDidMiniaturizeNotification, NSWindowDidDeminiaturizeNotification]) {
        [NSNotificationCenter.defaultCenter addObserver:guard selector:@selector(publishVisibility:) name:name object:window];
    }
    if (rememberFrame) {
        [window setFrameUsingName:@"OpenSurgeMainWindow"];
        // The first launch is deliberately spacious. Fit smaller displays and
        // frames restored from a larger monitor without overriding user sizing.
        NSRect frame = window.frame;
        NSRect available = (window.screen ?: NSScreen.mainScreen).visibleFrame;
        frame.size.width = MIN(frame.size.width, available.size.width);
        frame.size.height = MIN(frame.size.height, available.size.height);
        [window setFrame:[window constrainFrameRect:frame toScreen:window.screen] display:NO];
        [window setFrameAutosaveName:@"OpenSurgeMainWindow"];
    }
    [guard publishVisibility:nil];
}

void setOpenSurgeWindowLanguage(void *pointer, bool english) {
    NSWindow *window = (__bridge NSWindow *)pointer;
    WKWebView *webView = findWebView(window.contentView);
    OpenSurgeWebViewGuard *guard = objc_getAssociatedObject(webView, &guardKey);
    guard.english = english;
}

void setOpenSurgeWindowAppearance(void *pointer, bool dark) {
    NSWindow *window = (__bridge NSWindow *)pointer;
    window.appearance = [NSAppearance appearanceNamed:dark ? NSAppearanceNameDarkAqua : NSAppearanceNameAqua];
    WKWebView *webView = findWebView(window.contentView);
    OpenSurgeWebViewGuard *guard = objc_getAssociatedObject(webView, &guardKey);
    [guard publishAppearance:nil];
}

// Wails beta.26 scales every assigned image to NSStatusBar.thickness and does
// not expose its NSStatusItem. Status-item windows aren't in NSApp.windows.
// Intercept only NSStatusBarButton's public setter, and only for buttons owned
// by Wails' StatusItemController in this process. Install before tray creation.
// Keeping this narrow adapter here avoids a Wails fork or an inherited NSButton
// method replacement that would change ordinary controls.
void configureOpenSurgeMenuBarIcon(void) {
    static dispatch_once_t once;
    dispatch_once(&once, ^{
        Class buttonClass = NSStatusBarButton.class;
        SEL selector = @selector(setImage:);
        Method method = class_getInstanceMethod(buttonClass, selector);
        IMP original = method_getImplementation(method);
        IMP sized = imp_implementationWithBlock(^(NSStatusBarButton *button, NSImage *image) {
            Class controllerClass = NSClassFromString(@"StatusItemController");
            BOOL owned = controllerClass && [button.target isKindOfClass:controllerClass];
            if (owned && image) {
                image.size = NSMakeSize(18, 18);
                image.template = YES;
                button.imageScaling = NSImageScaleNone;
            }
            ((void (*)(id, SEL, NSImage *))original)(button, selector, image);
            if (owned && image && smokeActions()) NSLog(@"OpenSurge menu bar image: %.0f x %.0f pt; template=%d", button.image.size.width, button.image.size.height, button.image.isTemplate);
        });
        if (!class_addMethod(buttonClass, selector, sized, method_getTypeEncoding(method))) {
            method_setImplementation(class_getInstanceMethod(buttonClass, selector), sized);
        }
    });
}
