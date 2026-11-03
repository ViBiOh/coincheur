#import <AVFoundation/AVFoundation.h>
#include <stdlib.h>
#include <string.h>

#include "camera_darwin.h"

extern void cameraFrame(uintptr_t handle, void *data, int width, int height, int stride);

@interface FrameSink : NSObject <AVCaptureVideoDataOutputSampleBufferDelegate>
@property(nonatomic) uintptr_t handle;
@end

@implementation FrameSink

- (void)captureOutput:(AVCaptureOutput *)output
    didOutputSampleBuffer:(CMSampleBufferRef)sampleBuffer
           fromConnection:(AVCaptureConnection *)connection {
    CVImageBufferRef image = CMSampleBufferGetImageBuffer(sampleBuffer);
    if (image == NULL) {
        return;
    }

    CVPixelBufferLockBaseAddress(image, kCVPixelBufferLock_ReadOnly);
    cameraFrame(self.handle, CVPixelBufferGetBaseAddress(image), (int)CVPixelBufferGetWidth(image),
                (int)CVPixelBufferGetHeight(image), (int)CVPixelBufferGetBytesPerRow(image));
    CVPixelBufferUnlockBaseAddress(image, kCVPixelBufferLock_ReadOnly);
}

@end

static AVCaptureSession *session = nil;
static FrameSink *sink = nil;
static dispatch_queue_t queue = nil;

static int fail(char **errorMessage, int code, NSString *message) {
    *errorMessage = strdup([message UTF8String]);
    return code;
}

static int startLocked(uintptr_t handle, char **errorMessage);
static void stopLocked(void);

static BOOL requestAccess(void) {
    AVAuthorizationStatus status = [AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeVideo];
    if (status != AVAuthorizationStatusNotDetermined) {
        return status == AVAuthorizationStatusAuthorized;
    }

    dispatch_semaphore_t done = dispatch_semaphore_create(0);
    __block BOOL granted = NO;
    [AVCaptureDevice requestAccessForMediaType:AVMediaTypeVideo
                             completionHandler:^(BOOL allowed) {
                               granted = allowed;
                               dispatch_semaphore_signal(done);
                             }];
    dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);

    return granted;
}

int cameraStart(uintptr_t handle, char **errorMessage) {
    @synchronized([FrameSink class]) {
        return startLocked(handle, errorMessage);
    }
}

static int startLocked(uintptr_t handle, char **errorMessage) {
    if (session != nil) {
        return fail(errorMessage, 1, @"camera already started");
    }

    if (!requestAccess()) {
        return fail(errorMessage, 2, @"camera access denied, allow it in System Settings, Privacy & Security, Camera");
    }

    AVCaptureDevice *device = [AVCaptureDevice defaultDeviceWithMediaType:AVMediaTypeVideo];
    if (device == nil) {
        return fail(errorMessage, 3, @"no camera found");
    }

    NSError *error = nil;
    AVCaptureDeviceInput *input = [AVCaptureDeviceInput deviceInputWithDevice:device error:&error];
    if (input == nil) {
        return fail(errorMessage, 4, [NSString stringWithFormat:@"opening camera: %@", error.localizedDescription]);
    }

    AVCaptureSession *created = [[AVCaptureSession alloc] init];
    if ([created canSetSessionPreset:AVCaptureSessionPreset1280x720]) {
        created.sessionPreset = AVCaptureSessionPreset1280x720;
    }

    if (![created canAddInput:input]) {
        return fail(errorMessage, 5, @"camera input rejected by the session");
    }
    [created addInput:input];

    AVCaptureVideoDataOutput *output = [[AVCaptureVideoDataOutput alloc] init];
    output.videoSettings = @{(id)kCVPixelBufferPixelFormatTypeKey : @(kCVPixelFormatType_32BGRA)};
    output.alwaysDiscardsLateVideoFrames = YES;

    sink = [[FrameSink alloc] init];
    sink.handle = handle;
    queue = dispatch_queue_create("coincheur.camera", DISPATCH_QUEUE_SERIAL);
    [output setSampleBufferDelegate:sink queue:queue];

    if (![created canAddOutput:output]) {
        sink = nil;
        queue = nil;
        return fail(errorMessage, 6, @"camera output rejected by the session");
    }
    [created addOutput:output];

    [created startRunning];
    session = created;

    return 0;
}

void cameraStop(void) {
    @synchronized([FrameSink class]) {
        stopLocked();
    }
}

static void stopLocked(void) {
    if (session == nil) {
        return;
    }

    [session stopRunning];
    for (AVCaptureOutput *output in session.outputs) {
        if ([output isKindOfClass:[AVCaptureVideoDataOutput class]]) {
            [(AVCaptureVideoDataOutput *)output setSampleBufferDelegate:nil queue:NULL];
        }
    }
    // Waits for the frames already queued, so Go can safely forget the handle afterwards.
    dispatch_sync(queue, ^{
                  });

    session = nil;
    sink = nil;
    queue = nil;
}
