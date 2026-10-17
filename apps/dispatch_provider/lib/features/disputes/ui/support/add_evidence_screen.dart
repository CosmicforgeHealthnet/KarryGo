import '../../../../shared/widgets/dispatch_screen_frame.dart';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';

import '../../../../core/theme/theme_extensions.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 05 — Add Evidence
// Dashed upload zone · thumbnails · Next
// Returns List<String> of file paths to the caller.
// ═══════════════════════════════════════════════════════════════════════════

class AddEvidenceScreen extends StatefulWidget {
  const AddEvidenceScreen({super.key, this.initialPaths = const []});

  final List<String> initialPaths;

  @override
  State<AddEvidenceScreen> createState() => _AddEvidenceScreenState();
}

class _AddEvidenceScreenState extends State<AddEvidenceScreen> {
  final _picker = ImagePicker();
  late List<String> _paths = [...widget.initialPaths];

  Future<void> _pick() async {
    final x = await _picker.pickMultiImage(
      imageQuality: 85,
      maxWidth: 1920,
      maxHeight: 1920,
    );
    if (x.isEmpty || !mounted) return;
    setState(() {
      _paths = [..._paths, ...x.map((f) => f.path)];
    });
  }

  void _remove(int index) {
    setState(() => _paths = [..._paths]..removeAt(index));
  }

  void _done() {
    Navigator.of(context).pop(_paths);
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            // ── Header ──
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
              child: Row(
                children: [
                  DispatchBackButton(
                    onPressed: () => Navigator.of(context).pop(),
                  ),
                  const SizedBox(width: 4),
                  Expanded(
                    child: Text(
                      'Add evidence',
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                        color: c.text,
                        letterSpacing: -0.2,
                      ),
                    ),
                  ),
                ],
              ),
            ),

            // ── Step indicator ──
            const Padding(
              padding: EdgeInsets.fromLTRB(20, 12, 20, 16),
              child: _StepIndicator(step: 2),
            ),

            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 4, 20, 24),
                children: [
                  Text(
                    'Add photos, screenshots or documents to help us understand the issue.',
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w500,
                      color: c.textMuted,
                      height: 1.45,
                    ),
                  ),
                  const SizedBox(height: 24),

                  // ── Dashed upload zone ──
                  if (_paths.isEmpty)
                    GestureDetector(
                      onTap: _pick,
                      behavior: HitTestBehavior.opaque,
                      child: CustomPaint(
                        painter: _DashPainter(color: c.border),
                        child: Container(
                          width: double.infinity,
                          padding: const EdgeInsets.symmetric(
                            vertical: 48,
                            horizontal: 16,
                          ),
                          decoration: BoxDecoration(
                            color: c.surface,
                            borderRadius: BorderRadius.circular(16),
                          ),
                          child: Column(
                            children: [
                              Container(
                                width: 64,
                                height: 64,
                                alignment: Alignment.center,
                                decoration: BoxDecoration(
                                  color: c.teal10,
                                  shape: BoxShape.circle,
                                ),
                                child: Icon(
                                  Icons.upload_rounded,
                                  size: 30,
                                  color: c.brandTeal600,
                                ),
                              ),
                              const SizedBox(height: 14),
                              Text(
                                'Tap to upload',
                                style: TextStyle(
                                  fontSize: 15,
                                  fontWeight: FontWeight.w700,
                                  color: c.text,
                                ),
                              ),
                              const SizedBox(height: 4),
                              Text(
                                'Photos, PDF, up to 10 MB each',
                                style: TextStyle(
                                  fontSize: 12.5,
                                  color: c.textMuted,
                                ),
                              ),
                            ],
                          ),
                        ),
                      ),
                    )
                  else ...[
                    // ── Thumbnail grid ──
                    GridView.builder(
                      shrinkWrap: true,
                      physics: const NeverScrollableScrollPhysics(),
                      gridDelegate:
                          const SliverGridDelegateWithFixedCrossAxisCount(
                            crossAxisCount: 3,
                            crossAxisSpacing: 10,
                            mainAxisSpacing: 10,
                          ),
                      itemCount: _paths.length + 1,
                      itemBuilder: (context, i) {
                        if (i == _paths.length) {
                          return _AddMoreTile(onTap: _pick);
                        }
                        return _Thumb(
                          path: _paths[i],
                          onRemove: () => _remove(i),
                        );
                      },
                    ),
                    const SizedBox(height: 16),
                    Text(
                      '${_paths.length} attached',
                      style: TextStyle(
                        fontSize: 13,
                        fontWeight: FontWeight.w500,
                        color: c.textMuted,
                      ),
                    ),
                  ],
                ],
              ),
            ),

            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _done,
                  style: FilledButton.styleFrom(
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('Next'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Thumbnail + add-more tile
// ═══════════════════════════════════════════════════════════════════════════

class _Thumb extends StatelessWidget {
  const _Thumb({required this.path, required this.onRemove});
  final String path;
  final VoidCallback onRemove;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Stack(
      clipBehavior: Clip.none,
      children: [
        ClipRRect(
          borderRadius: BorderRadius.circular(10),
          child: Image.file(
            File(path),
            width: double.infinity,
            height: double.infinity,
            fit: BoxFit.cover,
            errorBuilder: (_, __, ___) => Container(
              color: c.navy10,
              alignment: Alignment.center,
              child: Icon(
                Icons.description_outlined,
                size: 26,
                color: c.textMuted,
              ),
            ),
          ),
        ),
        Positioned(
          top: -6,
          right: -6,
          child: GestureDetector(
            onTap: onRemove,
            child: Container(
              width: 22,
              height: 22,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: c.danger,
                shape: BoxShape.circle,
                border: Border.all(color: c.background, width: 2),
              ),
              child: const Icon(
                Icons.close_rounded,
                size: 12,
                color: Colors.white,
              ),
            ),
          ),
        ),
      ],
    );
  }
}

class _AddMoreTile extends StatelessWidget {
  const _AddMoreTile({required this.onTap});
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: c.navy10,
          borderRadius: BorderRadius.circular(10),
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.add_rounded, size: 26, color: c.brandTeal600),
            const SizedBox(height: 4),
            Text(
              'Add more',
              style: TextStyle(
                fontSize: 11.5,
                fontWeight: FontWeight.w600,
                color: c.brandTeal600,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _StepIndicator extends StatelessWidget {
  const _StepIndicator({required this.step});
  final int step;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: List.generate(3, (i) {
        final n = i + 1;
        final active = n <= step;
        return Padding(
          padding: const EdgeInsets.symmetric(horizontal: 6),
          child: AnimatedContainer(
            duration: const Duration(milliseconds: 200),
            width: active ? 28 : 10,
            height: 10,
            decoration: BoxDecoration(
              color: active ? c.brandTeal500 : c.navy20,
              borderRadius: BorderRadius.circular(999),
            ),
          ),
        );
      }),
    );
  }
}

class _DashPainter extends CustomPainter {
  const _DashPainter({required this.color});
  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 1.4
      ..style = PaintingStyle.stroke;

    final path = Path()
      ..addRRect(
        RRect.fromRectAndRadius(
          Rect.fromLTWH(0, 0, size.width, size.height),
          const Radius.circular(16),
        ),
      );

    for (final metric in path.computeMetrics()) {
      double d = 0;
      while (d < metric.length) {
        canvas.drawPath(metric.extractPath(d, d + 6), paint);
        d += 10;
      }
    }
  }

  @override
  bool shouldRepaint(_DashPainter old) => old.color != color;
}
