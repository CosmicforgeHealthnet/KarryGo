import 'dart:io';
import 'dart:ui' as ui;
import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import '../data/provider_operations_api.dart';

class SignatureCaptureScreen extends StatefulWidget {
  const SignatureCaptureScreen({super.key});
  @override
  State<SignatureCaptureScreen> createState() => _SignatureCaptureScreenState();
}

class _SignatureCaptureScreenState extends State<SignatureCaptureScreen> {
  final _canvas = GlobalKey();
  final _points = <Offset?>[];
  bool _saving = false;
  String? _error;
  Future<void> _save() async {
    if (_saving || _points.whereType<Offset>().length < 8) return;
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      final boundary =
          _canvas.currentContext!.findRenderObject() as RenderRepaintBoundary;
      final image = await boundary.toImage(pixelRatio: 2);
      final data = await image.toByteData(format: ui.ImageByteFormat.png);
      image.dispose();
      if (data == null) throw StateError('Signature is empty');
      final file = File(
        '${Directory.systemTemp.path}${Platform.pathSeparator}dispatch-signature-${ProviderOperationsApi.newOperationKey()}.png',
      );
      await file.writeAsBytes(data.buffer.asUint8List(), flush: true);
      if (mounted) Navigator.of(context).pop(file.path);
    } catch (_) {
      if (mounted)
        setState(() {
          _saving = false;
          _error = 'The signature could not be saved. Please try again.';
        });
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Recipient signature')),
    body: SafeArea(
      child: ListView(
        padding: const EdgeInsets.all(24),
        children: [
          Text(
            'Please ask the recipient to sign',
            style: Theme.of(context).textTheme.headlineSmall,
          ),
          const SizedBox(height: 12),
          const Text('Only continue with the recipient’s permission.'),
          const SizedBox(height: 24),
          ClipRRect(
            borderRadius: BorderRadius.circular(20),
            child: RepaintBoundary(
              key: _canvas,
              child: ColoredBox(
                color: Colors.white,
                child: SizedBox(
                  height: 240,
                  width: double.infinity,
                  child: Semantics(
                    label: 'Signature drawing area',
                    child: GestureDetector(
                      onPanStart: _saving
                          ? null
                          : (event) => setState(
                              () => _points.add(event.localPosition),
                            ),
                      onPanUpdate: _saving
                          ? null
                          : (event) => setState(
                              () => _points.add(event.localPosition),
                            ),
                      onPanEnd: _saving
                          ? null
                          : (_) => setState(() => _points.add(null)),
                      child: CustomPaint(
                        painter: _SignaturePainter(List.of(_points)),
                      ),
                    ),
                  ),
                ),
              ),
            ),
          ),
          const SizedBox(height: 12),
          TextButton.icon(
            onPressed: _saving ? null : () => setState(_points.clear),
            icon: const Icon(Icons.restart_alt_rounded),
            label: const Text('Clear signature'),
          ),
          if (_error != null)
            Text(
              _error!,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
          const SizedBox(height: 20),
          FilledButton(
            onPressed: _saving || _points.whereType<Offset>().length < 8
                ? null
                : _save,
            child: Text(_saving ? 'Saving...' : 'Use this signature'),
          ),
        ],
      ),
    ),
  );
}

class _SignaturePainter extends CustomPainter {
  _SignaturePainter(this.points);
  final List<Offset?> points;
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = const Color(0xff12233b)
      ..strokeWidth = 3
      ..strokeCap = StrokeCap.round;
    canvas.clipRect(Offset.zero & size);
    for (var i = 1; i < points.length; i++) {
      if (points[i - 1] != null && points[i] != null)
        canvas.drawLine(points[i - 1]!, points[i]!, paint);
    }
  }

  @override
  bool shouldRepaint(_SignaturePainter oldDelegate) => true;
}
