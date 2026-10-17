import '../../../shared/widgets/async_content.dart';
import '../../../core/app_mode.dart';
import '../../../shared/widgets/dispatch_screen_frame.dart';
import 'dart:io';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:image_picker/image_picker.dart';

import '../../../core/theme/theme_extensions.dart';

class PhotoUploadScreen extends StatefulWidget {
  const PhotoUploadScreen({
    super.key,
    required this.onContinue,
    required this.onBack,
    required this.currentStep,
    required this.totalSteps,
  });

  final Future<void> Function(String) onContinue;
  final VoidCallback onBack;
  final int currentStep;
  final int totalSteps;

  @override
  State<PhotoUploadScreen> createState() => _PhotoUploadScreenState();
}

class _PhotoUploadScreenState extends State<PhotoUploadScreen>
    with TickerProviderStateMixin {
  final ImagePicker _picker = ImagePicker();

  XFile? _pickedFile;
  bool _picking = false;
  bool _uploading = false;
  bool _confirmed = false;

  late final AnimationController _entrance;
  late final AnimationController _ring;

  bool get _busy => _picking || _uploading;

  @override
  void initState() {
    super.initState();
    _entrance = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1000),
    )..forward();
    _ring = AnimationController(
      vsync: this,
      duration: const Duration(seconds: 30),
    )..repeat();
    if (AppModeConfig.instance.isMock) {
      _loadDemoPhoto();
    } else {
      _recoverLostPhoto();
    }
  }

  @override
  void dispose() {
    _entrance.dispose();
    _ring.dispose();
    super.dispose();
  }

  Future<void> _loadDemoPhoto() async {
    try {
      final asset = await rootBundle.load('assets/figma/welcome_hero.png');
      final directory = await Directory.systemTemp.createTemp('karryam-demo-');
      final file = File('${directory.path}/rider.png');
      await file.writeAsBytes(
        asset.buffer.asUint8List(asset.offsetInBytes, asset.lengthInBytes),
      );
      if (!mounted) return;
      setState(() {
        _pickedFile = XFile(file.path);
        _confirmed = true;
      });
    } catch (_) {
      if (mounted)
        _showError(
          'Could not prepare the demo photo. Choose a photo to continue.',
        );
    }
  }

  Future<void> _recoverLostPhoto() async {
    if (!Platform.isAndroid) return;
    try {
      final LostDataResponse response = await _picker.retrieveLostData();
      if (response.isEmpty || !mounted) return;
      final XFile? file = response.file;
      if (file != null) {
        setState(() {
          _pickedFile = file;
          _confirmed = false;
        });
      }
    } catch (e) {
      debugPrint('retrieveLostData failed: $e');
    }
  }

  void _showError(String message) {
    if (mounted) showDispatchError(context, message);
  }

  String _pickerMessage(ImageSource source, String code) {
    final bool isCamera = source == ImageSource.camera;
    if (code == 'camera_access_denied' || code == 'photo_access_denied') {
      return isCamera
          ? 'Camera unavailable. Please grant camera permission in Settings.'
          : 'Could not access gallery. Please grant photo permission in Settings.';
    }
    return isCamera
        ? 'Could not open the camera. Please try again.'
        : 'Could not open the gallery. Please try again.';
  }

  Future<void> _pickImage(ImageSource source) async {
    if (_busy) return;
    setState(() => _picking = true);

    try {
      final XFile? file = await _picker.pickImage(
        source: source,
        imageQuality: 85,
        maxWidth: 1080,
        maxHeight: 1080,
        preferredCameraDevice: CameraDevice.front,
      );

      if (!mounted) return;

      if (file != null) {
        setState(() {
          _pickedFile = file;
          _confirmed = false;
        });
      }
    } on PlatformException catch (e) {
      debugPrint('pickImage PlatformException: ${e.code} ${e.message}');
      if (e.code != 'already_active') {
        _showError(_pickerMessage(source, e.code));
      }
    } catch (e) {
      debugPrint('pickImage failed: $e');
      _showError(_pickerMessage(source, ''));
    } finally {
      if (mounted) setState(() => _picking = false);
    }
  }

  void _confirmPhoto() {
    if (_busy || _pickedFile == null) return;
    setState(() => _confirmed = true);
  }

  Future<void> _handleContinue() async {
    final XFile? file = _pickedFile;
    if (file == null || !_confirmed || _busy) return;
    setState(() => _uploading = true);
    try {
      await widget.onContinue(file.path);
    } catch (e) {
      debugPrint('Photo upload failed: $e');
      final message = e.toString().replaceFirst('Exception: ', '');
      _showError(
        message.isEmpty
            ? 'Upload failed. Check your connection and try again.'
            : message,
      );
    } finally {
      if (mounted) setState(() => _uploading = false);
    }
  }

  void _handleBack() {
    if (_uploading) return;
    widget.onBack();
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final bool hasPhoto = _pickedFile != null;
    final bool canContinue = hasPhoto && _confirmed && !_busy;

    final VoidCallback? onCamera = _busy
        ? null
        : () => _pickImage(ImageSource.camera);
    final VoidCallback? onGallery = _busy
        ? null
        : () => _pickImage(ImageSource.gallery);
    final VoidCallback? onUse = _busy ? null : _confirmPhoto;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.symmetric(horizontal: 24),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    const SizedBox(height: 8),
                    _Reveal(
                      animation: _entrance,
                      begin: 0,
                      child: DispatchSetupHeader(
                        current: widget.currentStep,
                        total: widget.totalSteps,
                        onBack: _handleBack,
                      ),
                    ),
                    const SizedBox(height: 28),
                    _Reveal(
                      animation: _entrance,
                      begin: 0.08,
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          Text(
                            'Profile photo',
                            style: TextStyle(
                              fontSize: 28,
                              fontWeight: FontWeight.w700,
                              color: c.text,
                              letterSpacing: -0.4,
                              height: 1.2,
                            ),
                          ),
                          const SizedBox(height: 10),
                          Text(
                            'Take a clear photo of yourself. This helps build trust on the platform.',
                            style: TextStyle(
                              fontSize: 15,
                              fontWeight: FontWeight.w400,
                              color: c.textMuted,
                              height: 1.5,
                            ),
                          ),
                        ],
                      ),
                    ),
                    const SizedBox(height: 24),
                    _Reveal(
                      animation: _entrance,
                      begin: 0.2,
                      child: Center(
                        child: _PhotoCircle(
                          file: _pickedFile,
                          isLoading: _busy,
                          confirmed: _confirmed,
                          ring: _ring,
                          onTap: onCamera,
                        ),
                      ),
                    ),
                    const SizedBox(height: 20),

                    SizedBox(
                      height: 24,
                      child: Center(
                        child: AnimatedSwitcher(
                          duration: const Duration(milliseconds: 200),
                          child: hasPhoto
                              ? const SizedBox.shrink(key: ValueKey('no-tips'))
                              : const _PhotoTips(key: ValueKey('tips')),
                        ),
                      ),
                    ),
                    const SizedBox(height: 20),

                    _Reveal(
                      animation: _entrance,
                      begin: 0.4,
                      child: SizedBox(
                        height: 82,
                        child: Row(
                          crossAxisAlignment: CrossAxisAlignment.stretch,
                          children: hasPhoto
                              ? [
                                  Expanded(
                                    child: _ActionTile(
                                      icon: Icons.replay_rounded,
                                      label: 'Retake',
                                      onTap: onCamera,
                                    ),
                                  ),
                                  const SizedBox(width: 10),
                                  Expanded(
                                    child: _ActionTile(
                                      icon: Icons.photo_library_outlined,
                                      label: 'Gallery',
                                      onTap: onGallery,
                                    ),
                                  ),
                                  const SizedBox(width: 10),
                                  Expanded(
                                    child: _ActionTile(
                                      icon: Icons.check_rounded,
                                      label: 'Use photo',
                                      highlighted: true,
                                      filled: _confirmed,
                                      onTap: onUse,
                                    ),
                                  ),
                                ]
                              : [
                                  Expanded(
                                    child: _ActionTile(
                                      icon: Icons.photo_camera_outlined,
                                      label: 'Take photo',
                                      onTap: onCamera,
                                    ),
                                  ),
                                  const SizedBox(width: 12),
                                  Expanded(
                                    child: _ActionTile(
                                      icon: Icons.photo_library_outlined,
                                      label: 'Upload from gallery',
                                      onTap: onGallery,
                                    ),
                                  ),
                                ],
                        ),
                      ),
                    ),
                    const SizedBox(height: 24),
                  ],
                ),
              ),
            ),

            Padding(
              padding: const EdgeInsets.fromLTRB(24, 8, 24, 20),
              child: AnimatedContainer(
                duration: const Duration(milliseconds: 220),
                height: 58,
                width: double.infinity,
                decoration: BoxDecoration(
                  borderRadius: BorderRadius.circular(12),
                  boxShadow: canContinue
                      ? [
                          BoxShadow(
                            color: c.brandTeal500.withValues(alpha: 0.20),
                            blurRadius: 20,
                            offset: const Offset(0, 8),
                          ),
                        ]
                      : const [],
                ),
                child: FilledButton(
                  onPressed: canContinue ? _handleContinue : null,
                  style: FilledButton.styleFrom(
                    backgroundColor: c.brandTeal500,
                    disabledBackgroundColor: c.brandTeal500.withValues(
                      alpha: 0.45,
                    ),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(12),
                    ),
                  ),
                  child: _uploading
                      ? const SizedBox(
                          width: 22,
                          height: 22,
                          child: CircularProgressIndicator(
                            color: Color(0xFF081F44),
                            strokeWidth: 2,
                          ),
                        )
                      : const Text(
                          'Continue',
                          style: TextStyle(
                            fontSize: 15,
                            fontWeight: FontWeight.w700,
                            color: Color(0xFF081F44),
                          ),
                        ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ─── Entrance reveal ────────────────────────────────────────────────────────

class _Reveal extends StatelessWidget {
  const _Reveal({
    required this.animation,
    required this.begin,
    required this.child,
  });

  final Animation<double> animation;
  final double begin;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final end = begin + 0.45 > 1 ? 1.0 : begin + 0.45;
    final interval = Interval(begin, end, curve: Curves.easeOutCubic);

    return AnimatedBuilder(
      animation: animation,
      child: child,
      builder: (context, child) {
        final t = interval.transform(animation.value);
        return Opacity(
          opacity: t,
          child: Transform.translate(
            offset: Offset(0, (1 - t) * 16),
            child: child,
          ),
        );
      },
    );
  }
}

// ─── Photo circle with rotating dashed ring ─────────────────────────────────

class _PhotoCircle extends StatelessWidget {
  const _PhotoCircle({
    required this.file,
    required this.isLoading,
    required this.confirmed,
    required this.ring,
    required this.onTap,
  });

  final XFile? file;
  final bool isLoading;
  final bool confirmed;
  final Animation<double> ring;
  final VoidCallback? onTap;

  static const double _outer = 216;
  static const double _inset = 16;
  static const double _badge = 44;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    const radius = _outer / 2 - _inset;
    const badgeOffset = _outer / 2 + radius * 0.7071 - _badge / 2;

    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: SizedBox(
        width: _outer,
        height: _outer,
        child: Stack(
          children: [
            Positioned.fill(
              child: RepaintBoundary(
                child: TweenAnimationBuilder<double>(
                  tween: Tween(end: confirmed ? 0.0 : 0.55),
                  duration: const Duration(milliseconds: 500),
                  curve: Curves.easeOutCubic,
                  builder: (context, gap, _) {
                    return CustomPaint(
                      painter: _RingPainter(
                        rotation: ring,
                        gapRatio: gap,
                        color: c.brandTeal500,
                      ),
                    );
                  },
                ),
              ),
            ),
            Positioned.fill(
              child: Padding(
                padding: const EdgeInsets.all(_inset),
                child: RepaintBoundary(
                  child: Container(
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      boxShadow: [
                        BoxShadow(
                          color: c.brandNavy900.withValues(alpha: 0.15),
                          blurRadius: 24,
                          offset: const Offset(0, 10),
                        ),
                      ],
                    ),
                    child: ClipOval(
                      child: Container(
                        color: c.inputFill,
                        child: Stack(
                          fit: StackFit.expand,
                          children: [
                            AnimatedSwitcher(
                              duration: const Duration(milliseconds: 320),
                              transitionBuilder: (child, animation) {
                                return FadeTransition(
                                  opacity: animation,
                                  child: ScaleTransition(
                                    scale: Tween<double>(
                                      begin: 0.94,
                                      end: 1,
                                    ).animate(animation),
                                    child: child,
                                  ),
                                );
                              },
                              child: file != null
                                  ? SizedBox.expand(
                                      key: ValueKey(file!.path),
                                      child: Image.file(
                                        File(file!.path),
                                        fit: BoxFit.cover,
                                        cacheWidth: 648,
                                        gaplessPlayback: true,
                                        errorBuilder: (_, __, ___) =>
                                            const _EmptyPhoto(),
                                      ),
                                    )
                                  : const _EmptyPhoto(key: ValueKey('empty')),
                            ),
                            if (isLoading)
                              Container(
                                color: Colors.black.withValues(alpha: 0.25),
                                child: const Center(
                                  child: CircularProgressIndicator(
                                    color: Colors.white,
                                    strokeWidth: 3,
                                  ),
                                ),
                              ),
                          ],
                        ),
                      ),
                    ),
                  ),
                ),
              ),
            ),
            Positioned(
              left: badgeOffset,
              top: badgeOffset,
              child: _CameraBadge(confirmed: confirmed, size: _badge),
            ),
          ],
        ),
      ),
    );
  }
}

class _EmptyPhoto extends StatelessWidget {
  const _EmptyPhoto({super.key});

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return SizedBox.expand(
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Icon(Icons.person_outline_rounded, size: 72, color: c.textDisabled),
          const SizedBox(height: 4),
          Text(
            'Tap to take a photo',
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w500,
              color: c.textMuted,
            ),
          ),
        ],
      ),
    );
  }
}

class _CameraBadge extends StatelessWidget {
  const _CameraBadge({required this.confirmed, required this.size});

  final bool confirmed;
  final double size;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: c.brandTeal500,
        shape: BoxShape.circle,
        border: Border.all(color: c.background, width: 3),
        boxShadow: [
          BoxShadow(
            color: c.brandTeal500.withValues(alpha: 0.25),
            blurRadius: 12,
            offset: const Offset(0, 4),
          ),
        ],
      ),
      child: AnimatedSwitcher(
        duration: const Duration(milliseconds: 220),
        transitionBuilder: (child, animation) =>
            ScaleTransition(scale: animation, child: child),
        child: Icon(
          confirmed ? Icons.check_rounded : Icons.photo_camera_rounded,
          key: ValueKey(confirmed),
          size: 20,
          color: Colors.white,
        ),
      ),
    );
  }
}

class _RingPainter extends CustomPainter {
  _RingPainter({
    required this.rotation,
    required this.gapRatio,
    required this.color,
  }) : super(repaint: rotation);

  final Animation<double> rotation;
  final double gapRatio;
  final Color color;

  static const int _dashCount = 40;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2.4
      ..strokeCap = StrokeCap.round;

    final center = Offset(size.width / 2, size.height / 2);
    final radius = size.width / 2 - 2;
    final rect = Rect.fromCircle(center: center, radius: radius);

    final circumference = 2 * math.pi * radius;
    final dashLength = circumference / _dashCount;
    final sweep = (dashLength * (1 - gapRatio)) / radius;
    final step = (2 * math.pi) / _dashCount;
    final start = rotation.value * 2 * math.pi;

    final path = Path();
    for (int i = 0; i < _dashCount; i++) {
      path.addArc(rect, start + i * step, sweep);
    }
    canvas.drawPath(path, paint);
  }

  @override
  bool shouldRepaint(_RingPainter oldDelegate) =>
      oldDelegate.rotation != rotation ||
      oldDelegate.gapRatio != gapRatio ||
      oldDelegate.color != color;
}

// ─── Tips ───────────────────────────────────────────────────────────────────

class _PhotoTips extends StatelessWidget {
  const _PhotoTips({super.key});

  @override
  Widget build(BuildContext context) {
    return const FittedBox(
      fit: BoxFit.scaleDown,
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          _Tip(icon: Icons.wb_sunny_outlined, label: 'Good light'),
          SizedBox(width: 20),
          _Tip(icon: Icons.face_outlined, label: 'Face forward'),
          SizedBox(width: 20),
          _Tip(icon: Icons.visibility_outlined, label: 'Clear view'),
        ],
      ),
    );
  }
}

class _Tip extends StatelessWidget {
  const _Tip({required this.icon, required this.label});

  final IconData icon;
  final String label;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(icon, size: 16, color: c.brandTeal500),
        const SizedBox(width: 6),
        Text(
          label,
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w500,
            color: c.textMuted,
          ),
        ),
      ],
    );
  }
}

// ─── Action tile ────────────────────────────────────────────────────────────

class _ActionTile extends StatefulWidget {
  const _ActionTile({
    required this.icon,
    required this.label,
    required this.onTap,
    this.highlighted = false,
    this.filled = false,
  });

  final IconData icon;
  final String label;
  final VoidCallback? onTap;
  final bool highlighted;
  final bool filled;

  @override
  State<_ActionTile> createState() => _ActionTileState();
}

class _ActionTileState extends State<_ActionTile> {
  bool _pressed = false;

  void _setPressed(bool value) {
    if (_pressed == value || !mounted) return;
    setState(() => _pressed = value);
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final bool enabled = widget.onTap != null;
    final Color foreground = widget.filled
        ? Colors.white
        : (widget.highlighted ? c.brandTeal500 : c.text);
    final Color background = widget.filled ? c.brandTeal500 : c.surface;
    final Color border = widget.filled || widget.highlighted
        ? c.brandTeal500
        : c.border;

    return Semantics(
      button: true,
      enabled: enabled,
      label: widget.label,
      excludeSemantics: true,
      child: GestureDetector(
        onTap: enabled
            ? () {
                HapticFeedback.lightImpact();
                widget.onTap!();
              }
            : null,
        onTapDown: enabled ? (_) => _setPressed(true) : null,
        onTapUp: (_) => _setPressed(false),
        onTapCancel: () => _setPressed(false),
        behavior: HitTestBehavior.opaque,
        child: AnimatedScale(
          scale: _pressed ? 0.97 : 1,
          duration: const Duration(milliseconds: 110),
          child: AnimatedOpacity(
            opacity: enabled ? 1 : 0.5,
            duration: const Duration(milliseconds: 150),
            child: AnimatedContainer(
              duration: const Duration(milliseconds: 220),
              curve: Curves.easeOut,
              decoration: BoxDecoration(
                color: background,
                borderRadius: BorderRadius.circular(16),
                border: Border.all(color: border, width: 1.2),
                boxShadow: [
                  BoxShadow(
                    color: widget.filled
                        ? c.brandTeal500.withValues(alpha: 0.20)
                        : c.brandNavy900.withValues(alpha: 0.04),
                    blurRadius: widget.filled ? 16 : 8,
                    offset: Offset(0, widget.filled ? 6 : 2),
                  ),
                ],
              ),
              child: Center(
                child: Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 8),
                  child: AnimatedSwitcher(
                    duration: const Duration(milliseconds: 180),
                    child: Column(
                      key: ValueKey('${widget.label}|${widget.filled}'),
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Icon(widget.icon, size: 24, color: foreground),
                        const SizedBox(height: 6),
                        FittedBox(
                          fit: BoxFit.scaleDown,
                          child: Text(
                            widget.label,
                            maxLines: 1,
                            textAlign: TextAlign.center,
                            style: TextStyle(
                              fontSize: 14,
                              fontWeight: FontWeight.w600,
                              color: foreground,
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

// ─── Step Progress ──────────────────────────────────────────────────────────
