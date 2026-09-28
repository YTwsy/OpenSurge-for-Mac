#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <objc/runtime.h>

static const char guardKey;

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
@interface OpenSurgeWebViewGuard : NSObject <WKNavigationDelegate, WKUIDelegate>
@property (weak) id<WKNavigationDelegate> navigation;
@property (weak) id<WKUIDelegate> ui;
@property (weak) WKWebView *webView;
@property BOOL english;
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
- (void)publishVisibility:(NSNotification *)notification {
    NSWindow *window = self.webView.window;
    BOOL visible = window.isVisible && !window.isMiniaturized && (window.occlusionState & NSWindowOcclusionStateVisible);
    NSString *script = [NSString stringWithFormat:@"window.__opensurgeWindowVisible=%@;document.dispatchEvent(new Event('visibilitychange'));", visible ? @"true" : @"false"];
    [self.webView evaluateJavaScript:script completionHandler:nil];
}
- (void)webView:(WKWebView *)webView didFinishNavigation:(WKNavigation *)navigation {
    if ([self.navigation respondsToSelector:_cmd]) [self.navigation webView:webView didFinishNavigation:navigation];
    [self publishVisibility:nil];
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
