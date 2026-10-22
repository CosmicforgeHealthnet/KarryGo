import 'dart:io';
import 'dart:math' as math;
import 'dart:ui' as ui;

/// Local checks only. They reject empty, corrupt and uniformly blank images;
/// they do NOT replace Smile ID liveness, identity or face matching.
class SelfieImageQuality {
  const SelfieImageQuality._(this.problem);

  final String? problem;
  bool get isUsable => problem == null;

  static Future<SelfieImageQuality> check(String path) async {
    try {
      final file = File(path);
      if (!await file.exists()) {
        return const SelfieImageQuality._('The captured selfie is missing. Retake it.');
      }
      final size = await file.length();
      if (size < 1024) {
        return const SelfieImageQuality._('The camera returned an empty photo. Retake it.');
      }
      if (size > 3 * 1024 * 1024) {
        return const SelfieImageQuality._(
          'The selfie is too large. Retake it using a lower resolution.',
        );
      }

      final bytes = await file.readAsBytes();
      final codec = await ui.instantiateImageCodec(bytes);
      try {
        final frame = await codec.getNextFrame();
        final image = frame.image;
        try {
          if (image.width < 240 || image.height < 240) {
            return const SelfieImageQuality._(
              'The selfie is too small. Move closer and retake it.',
            );
          }
          final raw = await image.toByteData(format: ui.ImageByteFormat.rawRgba);
          if (raw == null || raw.lengthInBytes < 4) {
            return const SelfieImageQuality._('The photo cannot be read. Retake it.');
          }
          final stride = math.max(1, (raw.lengthInBytes ~/ 4) ~/ 6000);
          var count = 0;
          var sum = 0.0;
          var sumSquares = 0.0;
          for (var i = 0; i + 3 < raw.lengthInBytes; i += stride * 4) {
            final brightness =
                (raw.getUint8(i) * 0.2126) +
                (raw.getUint8(i + 1) * 0.7152) +
                (raw.getUint8(i + 2) * 0.0722);
            count++;
            sum += brightness;
            sumSquares += brightness * brightness;
          }
          if (count == 0) {
            return const SelfieImageQuality._('The photo is empty. Retake it.');
          }
          final mean = sum / count;
          final variance = math.max(0.0, sumSquares / count - mean * mean);
          if (mean < 30) {
            return const SelfieImageQuality._(
              'The photo is too dark. Face a light source and retake it.',
            );
          }
          if (mean > 245) {
            return const SelfieImageQuality._(
              'The photo is overexposed. Avoid direct glare and retake it.',
            );
          }
          if (variance < 100) {
            return const SelfieImageQuality._(
              'The photo appears blank or out of focus. Retake it with your face visible.',
            );
          }
          return const SelfieImageQuality._(null);
        } finally {
          image.dispose();
        }
      } finally {
        codec.dispose();
      }
    } catch (_) {
      return const SelfieImageQuality._(
        'The photo is unreadable. Retake the selfie.',
      );
    }
  }
}
