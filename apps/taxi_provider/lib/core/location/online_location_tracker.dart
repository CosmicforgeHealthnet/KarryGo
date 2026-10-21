import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:geolocator/geolocator.dart';

/// Keeps the driver reachable while they are online.
///
/// Android suspends a backgrounded app within a minute or two, which stops the
/// heartbeat timer; the server then drops the driver from matching ~90 s later
/// while the app still says "Online". Streaming location as an Android
/// foreground service (a persistent "You're online" notification, with a wake
/// lock) keeps the app running with the screen off, and every fix is also a
/// chance to send a heartbeat the timer missed.
class OnlineLocationTracker {
  OnlineLocationTracker({required this.appName});

  /// Shown in the persistent notification.
  final String appName;

  StreamSubscription<Position>? _sub;
  Position? _last;

  bool get isRunning => _sub != null;

  /// The latest fix, when it is fresh enough to send as the driver's position.
  Position? get recentPosition {
    final p = _last;
    if (p == null) return null;
    if (DateTime.now().difference(p.timestamp) > const Duration(seconds: 60)) return null;
    return p;
  }

  /// Starts streaming; [onFix] runs for every new position. Does nothing when
  /// location permission isn't granted (going online asks for it first).
  Future<void> start({required void Function() onFix}) async {
    if (_sub != null) return;
    try {
      final permission = await Geolocator.checkPermission();
      if (permission == LocationPermission.denied || permission == LocationPermission.deniedForever) return;
      if (_sub != null) return;
      _sub = Geolocator.getPositionStream(locationSettings: _settings()).listen(
        (p) {
          _last = p;
          onFix();
        },
        // A dropped GPS fix must never take the driver offline; the heartbeat
        // timer and its Lagos fallback still run.
        onError: (Object _) {},
      );
    } catch (_) {
      // No location plugin (tests) or the service could not start.
    }
  }

  void stop() {
    _sub?.cancel();
    _sub = null;
  }

  LocationSettings _settings() {
    if (defaultTargetPlatform == TargetPlatform.android) {
      return AndroidSettings(
        accuracy: LocationAccuracy.high,
        distanceFilter: 0,
        intervalDuration: const Duration(seconds: 15),
        foregroundNotificationConfig: ForegroundNotificationConfig(
          notificationTitle: "You're online",
          notificationText: '$appName is sharing your location so you can receive requests.',
          notificationChannelName: 'Online status',
          enableWakeLock: true,
          setOngoing: true,
          notificationIcon: const AndroidResource(name: 'ic_launcher', defType: 'mipmap'),
        ),
      );
    }
    return const LocationSettings(accuracy: LocationAccuracy.high, distanceFilter: 0);
  }
}
