import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:dispatch_provider/features/profile/ui/selfie_image_quality.dart';

Future<File> _flatImage(Directory dir, Color color) async {
  final recorder = ui.PictureRecorder();
  final canvas = ui.Canvas(recorder);
  canvas.drawRect(
    const Rect.fromLTWH(0, 0, 300, 300),
    Paint()..color = color,
  );
  final picture = recorder.endRecording();
  final image = await picture.toImage(300, 300);
  try {
    final encoded = await image.toByteData(format: ui.ImageByteFormat.png);
    final file = File('${dir.path}/selfie.png');
    await file.writeAsBytes(encoded!.buffer.asUint8List());
    return file;
  } finally {
    image.dispose();
    picture.dispose();
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  late Directory temp;
  setUp(() async {
    temp = await Directory.systemTemp.createTemp('dispatch-selfie-');
  });
  tearDown(() async {
    await temp.delete(recursive: true);
  });

  test('missing photo fails safely', () async {
    final result = await SelfieImageQuality.check('${temp.path}/missing.jpg');
    expect(result.isUsable, false);
  });

  test('unreadable photo fails safely', () async {
    final file = File('${temp.path}/bad.jpg');
    await file.writeAsString('not a photo, only junk data');
    final result = await SelfieImageQuality.check(file.path);
    expect(result.isUsable, false);
  });

  test('uniform grey photo is rejected before remote verification', () async {
    final file = await _flatImage(temp, const Color(0xFF777777));
    final result = await SelfieImageQuality.check(file.path);
    expect(result.isUsable, false);
    expect(result.problem, contains('blank'));
  });

  test('uniform dark photo is rejected', () async {
    final file = await _flatImage(temp, const Color(0xFF0A0A0A));
    final result = await SelfieImageQuality.check(file.path);
    expect(result.isUsable, false);
  });
}
