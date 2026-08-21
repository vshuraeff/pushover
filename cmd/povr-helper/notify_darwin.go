//go:build darwin && cgo

// this backend must run as the executable of the installed povr.app bundle.
// bare go run and go test processes do not have the bundle identity required by
// unusernotificationcenter and must not exercise this file.
package main

/*
#cgo CFLAGS: -x objective-c -fblocks -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Foundation -framework UserNotifications -framework CoreFoundation

#include <CoreFoundation/CoreFoundation.h>
#include <Foundation/Foundation.h>
#include <UserNotifications/UserNotifications.h>
#include <objc/runtime.h>
#include <stdatomic.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static UNUserNotificationCenter *povr_center;
static id povr_delegate;
static NSMutableArray<NSData *> *povr_events;
static NSLock *povr_events_lock;
static CFRunLoopRef povr_main_run_loop;
static _Atomic bool povr_time_sensitive_allowed;
static NSString *const povr_emergency_category = @"povr.emergency";
static NSString *const povr_ack_action = @"povr.ack";

static NSString *povr_string(NSDictionary *dictionary, NSString *key) {
    id value = [dictionary objectForKey:key];
    if ([value isKindOfClass:[NSString class]]) {
        return (NSString *)value;
    }
    return @"";
}

static void povr_enqueue_event(NSDictionary *event) {
    @autoreleasepool {
        NSError *error = nil;
        NSData *data = [NSJSONSerialization dataWithJSONObject:event options:0 error:&error];
        if (data == nil) {
            fprintf(stderr, "povr-helper: encode event: %s\n", error.localizedDescription.UTF8String);
            return;
        }
        [povr_events_lock lock];
        [povr_events addObject:data];
        [povr_events_lock unlock];
        if (povr_main_run_loop != NULL) {
            CFRunLoopWakeUp(povr_main_run_loop);
        }
    }
}

static void povr_did_receive(
    id self,
    SEL selector,
    UNUserNotificationCenter *center,
    UNNotificationResponse *response,
    void (^completion_handler)(void)
) {
    (void)self;
    (void)selector;
    (void)center;
    @autoreleasepool {
        NSDictionary *user_info = response.notification.request.content.userInfo;
        NSString *identifier = povr_string(user_info, @"id");
        NSString *action_identifier = response.actionIdentifier;
        if ([action_identifier isEqualToString:UNNotificationDefaultActionIdentifier]) {
            NSString *url = povr_string(user_info, @"url");
            NSMutableDictionary *event = [NSMutableDictionary dictionaryWithObjectsAndKeys:
                @"clicked", @"event",
                identifier, @"id",
                nil];
            if (url.length > 0) {
                [event setObject:url forKey:@"url"];
            }
            povr_enqueue_event(event);
        } else if ([action_identifier isEqualToString:povr_ack_action]) {
            NSString *thread = response.notification.request.content.threadIdentifier;
            NSMutableDictionary *event = [NSMutableDictionary dictionaryWithObjectsAndKeys:
                @"action", @"event",
                @"ack", @"action",
                identifier, @"id",
                nil];
            if (thread.length > 0) {
                [event setObject:thread forKey:@"thread"];
            }
            povr_enqueue_event(event);
        }
    }
    completion_handler();
}

static void povr_will_present(
    id self,
    SEL selector,
    UNUserNotificationCenter *center,
    UNNotification *notification,
    void (^completion_handler)(UNNotificationPresentationOptions)
) {
    (void)self;
    (void)selector;
    (void)center;
    (void)notification;
    UNNotificationPresentationOptions options =
        UNNotificationPresentationOptionSound |
        UNNotificationPresentationOptionAlert;
    completion_handler(options);
}

static int povr_initialize(void) {
    @autoreleasepool {
        @try {
            povr_events = [[NSMutableArray alloc] init];
            povr_events_lock = [[NSLock alloc] init];
            povr_main_run_loop = CFRunLoopGetCurrent();
            CFRetain(povr_main_run_loop);
            atomic_store(&povr_time_sensitive_allowed, false);

            Class delegate_class = objc_allocateClassPair(
                [NSObject class],
                "PovrUserNotificationCenterDelegate",
                0
            );
            if (delegate_class != Nil) {
                class_addProtocol(delegate_class, @protocol(UNUserNotificationCenterDelegate));
                class_addMethod(
                    delegate_class,
                    @selector(userNotificationCenter:didReceiveNotificationResponse:withCompletionHandler:),
                    (IMP)povr_did_receive,
                    "v@:@@@?"
                );
                class_addMethod(
                    delegate_class,
                    @selector(userNotificationCenter:willPresentNotification:withCompletionHandler:),
                    (IMP)povr_will_present,
                    "v@:@@@?"
                );
                objc_registerClassPair(delegate_class);
            } else {
                delegate_class = objc_getClass("PovrUserNotificationCenterDelegate");
            }

            povr_delegate = [[delegate_class alloc] init];
            povr_center = [[UNUserNotificationCenter currentNotificationCenter] retain];
            if (povr_delegate == nil || povr_center == nil) {
                return -1;
            }
            povr_center.delegate = povr_delegate;
            UNNotificationAction *ack_action = [UNNotificationAction
                actionWithIdentifier:povr_ack_action
                title:@"Acknowledge"
                options:0];
            UNNotificationCategory *emergency_category = [UNNotificationCategory
                categoryWithIdentifier:povr_emergency_category
                actions:@[ack_action]
                intentIdentifiers:@[]
                options:0];
            [povr_center setNotificationCategories:[NSSet setWithObject:emergency_category]];
            return 0;
        } @catch (NSException *exception) {
            fprintf(stderr, "povr-helper: initialize notification center: %s\n", exception.reason.UTF8String);
            return -1;
        }
    }
}

static void povr_request_authorization(void) {
    UNAuthorizationOptions options = UNAuthorizationOptionAlert | UNAuthorizationOptionSound;
    if (@available(macOS 12.0, *)) {
        options |= UNAuthorizationOptionTimeSensitive;
    }

    [povr_center requestAuthorizationWithOptions:options completionHandler:^(BOOL granted, NSError *error) {
        [povr_center getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *settings) {
            @autoreleasepool {
                BOOL time_sensitive = NO;
                if (@available(macOS 12.0, *)) {
                    time_sensitive = settings.timeSensitiveSetting == UNNotificationSettingEnabled;
                }
                atomic_store(&povr_time_sensitive_allowed, time_sensitive);

                NSMutableDictionary *event = [NSMutableDictionary dictionaryWithObjectsAndKeys:
                    @"ready", @"event",
                    [NSNumber numberWithBool:granted], @"granted",
                    [NSNumber numberWithBool:time_sensitive], @"timeSensitive",
                    nil];
                if (error != nil) {
                    [event setObject:error.localizedDescription forKey:@"error"];
                }
                povr_enqueue_event(event);
            }
        }];
    }];
}

static int povr_post_json(const char *request_json) {
    @autoreleasepool {
        NSData *data = [NSData dataWithBytes:request_json length:strlen(request_json)];
        NSError *json_error = nil;
        id object = [NSJSONSerialization JSONObjectWithData:data options:0 error:&json_error];
        if (![object isKindOfClass:[NSDictionary class]]) {
            fprintf(stderr, "povr-helper: decode request: %s\n", json_error.localizedDescription.UTF8String);
            return -1;
        }

        NSDictionary *request = (NSDictionary *)object;
        NSString *identifier = povr_string(request, @"id");
        UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
        content.title = povr_string(request, @"title");
        content.subtitle = povr_string(request, @"subtitle");
        content.body = povr_string(request, @"body");
        content.threadIdentifier = povr_string(request, @"thread");

        NSString *category = povr_string(request, @"category");
        if (category.length > 0) {
            content.categoryIdentifier = category;
        }

        NSString *url = povr_string(request, @"url");
        NSMutableDictionary *user_info = [NSMutableDictionary dictionaryWithObject:identifier forKey:@"id"];
        if (url.length > 0) {
            [user_info setObject:url forKey:@"url"];
        }
        content.userInfo = user_info;

        BOOL level_fallback = NO;
        NSString *level = povr_string(request, @"level");
        if (@available(macOS 12.0, *)) {
            if ([level isEqualToString:@"passive"]) {
                content.interruptionLevel = UNNotificationInterruptionLevelPassive;
            } else if ([level isEqualToString:@"timeSensitive"]) {
                if (atomic_load(&povr_time_sensitive_allowed)) {
                    content.interruptionLevel = UNNotificationInterruptionLevelTimeSensitive;
                } else {
                    content.interruptionLevel = UNNotificationInterruptionLevelActive;
                    level_fallback = YES;
                    fprintf(stderr, "povr-helper: time-sensitive authorization unavailable; using active level for %s\n", identifier.UTF8String);
                }
            } else {
                content.interruptionLevel = UNNotificationInterruptionLevelActive;
            }
        } else if ([level isEqualToString:@"timeSensitive"]) {
            level_fallback = YES;
            fprintf(stderr, "povr-helper: time-sensitive level requires macos 12; using active level for %s\n", identifier.UTF8String);
        }

        BOOL sound_fallback = NO;
        id sound_object = [request objectForKey:@"sound"];
        if ([sound_object isKindOfClass:[NSDictionary class]]) {
            NSDictionary *sound = (NSDictionary *)sound_object;
            NSString *name = povr_string(sound, @"name");
            NSString *file = povr_string(sound, @"file");
            if (name.length > 0) {
                content.sound = [UNNotificationSound soundNamed:name];
            } else if (file.length > 0) {
                // usernotifications resolves named files from sound directories or the app bundle;
                // it cannot play an arbitrary source path directly.
                content.sound = [UNNotificationSound defaultSound];
                sound_fallback = YES;
                fprintf(stderr, "povr-helper: arbitrary sound path unsupported; using default sound for %s\n", identifier.UTF8String);
            }
        }

        NSString *attachment_path = povr_string(request, @"attachment");
        if (attachment_path.length == 0) {
            attachment_path = povr_string(request, @"icon");
        }
        if (attachment_path.length > 0) {
            NSError *attachment_error = nil;
            UNNotificationAttachment *attachment = [UNNotificationAttachment
                attachmentWithIdentifier:(identifier.length > 0 ? identifier : @"attachment")
                URL:[NSURL fileURLWithPath:attachment_path]
                options:nil
                error:&attachment_error];
            if (attachment != nil) {
                content.attachments = [NSArray arrayWithObject:attachment];
            } else {
                fprintf(
                    stderr,
                    "povr-helper: attach %s: %s\n",
                    attachment_path.UTF8String,
                    attachment_error.localizedDescription.UTF8String
                );
            }
        }

        NSString *request_identifier = identifier.length > 0
            ? identifier
            : [NSUUID UUID].UUIDString;
        UNNotificationRequest *notification_request = [UNNotificationRequest
            requestWithIdentifier:request_identifier
            content:content
            trigger:nil];

        [povr_center addNotificationRequest:notification_request withCompletionHandler:^(NSError *error) {
            @autoreleasepool {
                NSMutableDictionary *event = [NSMutableDictionary dictionaryWithObject:identifier forKey:@"id"];
                if (error != nil) {
                    [event setObject:@"error" forKey:@"event"];
                    [event setObject:error.localizedDescription forKey:@"error"];
                } else {
                    [event setObject:@"delivered" forKey:@"event"];
                }
                if (level_fallback) {
                    [event setObject:@YES forKey:@"levelFallback"];
                }
                if (sound_fallback) {
                    [event setObject:@YES forKey:@"soundFallback"];
                }
                povr_enqueue_event(event);
            }
        }];

        [content release];
        return 0;
    }
}

static char *povr_next_event(void) {
    [povr_events_lock lock];
    if (povr_events.count == 0) {
        [povr_events_lock unlock];
        return NULL;
    }
    NSData *data = [[povr_events objectAtIndex:0] retain];
    [povr_events removeObjectAtIndex:0];
    [povr_events_lock unlock];

    char *result = malloc(data.length + 1);
    if (result != NULL) {
        memcpy(result, data.bytes, data.length);
        result[data.length] = '\0';
    }
    [data release];
    return result;
}

static void povr_run_loop_once(double seconds) {
    CFRunLoopRunInMode(kCFRunLoopDefaultMode, seconds, true);
}
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"time"
	"unsafe"
)

const onceTimeout = 10 * time.Second

func run(stdin io.Reader, stdout, stderr io.Writer, once bool) int {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if C.povr_initialize() != 0 {
		_ = emitEvent(stdout, helperEvent{Event: "error", Error: "initialize notification center"})
		return 1
	}
	C.povr_request_authorization()

	results := make(chan requestResult)
	go readRequestLines(stdin, once, results)

	ready := false
	inputOpen := true
	haveRequest := false
	queued := make([]helperRequest, 0, 1)
	posted := make(map[string]struct{})
	var onceID string
	var deadline time.Time
	terminalExit := -1

	post := func(request helperRequest) {
		if err := postNative(request); err != nil {
			if emitErr := emitEvent(stdout, helperEvent{Event: "error", ID: request.ID, Error: err.Error()}); emitErr != nil {
				fmt.Fprintf(stderr, "povr-helper: write error event: %v\n", emitErr)
			}
			if once && request.ID == onceID {
				terminalExit = 1
			}
			return
		}
		posted[request.ID] = struct{}{}
	}

	for {
		for {
			raw := nextNativeEvent()
			if raw == nil {
				break
			}

			var event helperEvent
			if err := json.Unmarshal(raw, &event); err != nil {
				fmt.Fprintf(stderr, "povr-helper: decode native event: %v\n", err)
				continue
			}
			if err := writeJSONLine(stdout, raw); err != nil {
				fmt.Fprintf(stderr, "povr-helper: write event: %v\n", err)
				return 1
			}

			switch event.Event {
			case "ready":
				ready = true
				for _, request := range queued {
					post(request)
				}
				queued = queued[:0]
			case "delivered", "error":
				delete(posted, event.ID)
				if once && haveRequest && event.ID == onceID {
					if event.Event == "delivered" {
						return 0
					}
					return 1
				}
			}
		}

		if terminalExit >= 0 {
			return terminalExit
		}

	inputLoop:
		for inputOpen {
			select {
			case result, ok := <-results:
				if !ok {
					inputOpen = false
					break inputLoop
				}
				if result.err != nil {
					if err := emitEvent(stdout, helperEvent{Event: "error", Error: result.err.Error()}); err != nil {
						fmt.Fprintf(stderr, "povr-helper: write request error: %v\n", err)
						return 1
					}
					if once {
						return 1
					}
					continue
				}

				haveRequest = true
				if once {
					onceID = result.request.ID
					deadline = time.Now().Add(onceTimeout)
				}
				if ready {
					post(result.request)
				} else {
					queued = append(queued, result.request)
				}
			default:
				break inputLoop
			}
		}

		if terminalExit >= 0 {
			return terminalExit
		}
		if !inputOpen && !haveRequest && once {
			_ = emitEvent(stdout, helperEvent{Event: "error", Error: "no request received"})
			return 1
		}
		if !inputOpen && len(queued) == 0 && len(posted) == 0 {
			return 0
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			_ = emitEvent(stdout, helperEvent{Event: "error", ID: onceID, Error: "notification delivery timed out"})
			return 1
		}

		C.povr_run_loop_once(0.05)
	}
}

func postNative(request helperRequest) error {
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	cRequest := C.CString(string(data))
	defer C.free(unsafe.Pointer(cRequest))
	if C.povr_post_json(cRequest) != 0 {
		return errors.New("native notification request rejected")
	}
	return nil
}

func nextNativeEvent() []byte {
	cEvent := C.povr_next_event()
	if cEvent == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(cEvent))
	return []byte(C.GoString(cEvent))
}

func writeJSONLine(w io.Writer, raw []byte) error {
	line := make([]byte, len(raw)+1)
	copy(line, raw)
	line[len(raw)] = '\n'
	_, err := w.Write(line)
	return err
}
