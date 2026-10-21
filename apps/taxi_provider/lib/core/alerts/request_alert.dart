import 'dart:async';
import 'dart:io' show Platform;

import 'package:flutter_ringtone_player/flutter_ringtone_player.dart';
import 'package:vibration/vibration.dart';

/// Rings and vibrates while a new request waits for the driver's answer.
///
/// A request expires after its countdown, so a silent one is a missed job.
/// The ring loops until [stop] (answered, gone, or the driver went offline)
/// or [maxDuration], whichever comes first.
class RequestAlert {
  RequestAlert({this.maxDuration = const Duration(seconds: 30)});

  final Duration maxDuration;
  final _player = FlutterRingtonePlayer();
  Timer? _cap;
  bool _ringing = false;

  bool get isRinging => _ringing;

  /// Only phones ring; tests and desktop hosts have no ringer or vibrator.
  static bool get _supported => Platform.isAndroid || Platform.isIOS;

  Future<void> start() async {
    if (_ringing || !_supported) return;
    _ringing = true;
    _cap = Timer(maxDuration, stop);
    try {
      await _player.playRingtone(looping: true);
    } catch (_) {
      // No sound plugin (tests) or the device refused; vibration still runs.
    }
    try {
      if (await Vibration.hasVibrator()) {
        await Vibration.vibrate(pattern: const [0, 700, 500, 700, 1200], repeat: 0);
      }
    } catch (_) {}
  }

  Future<void> stop() async {
    if (!_ringing) return;
    _ringing = false;
    _cap?.cancel();
    _cap = null;
    try {
      await _player.stop();
    } catch (_) {}
    try {
      await Vibration.cancel();
    } catch (_) {}
  }
}
