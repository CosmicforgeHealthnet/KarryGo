import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../core/app_mode.dart';
import '../../../core/theme/theme_extensions.dart';
import '../state/wallet_controller.dart';

class ChangeTransactionPinScreen extends StatefulWidget {
  const ChangeTransactionPinScreen({
    super.key,
    required this.walletController,
  });

  final WalletController walletController;

  @override
  State<ChangeTransactionPinScreen> createState() =>
      _ChangeTransactionPinScreenState();
}

class _ChangeTransactionPinScreenState
    extends State<ChangeTransactionPinScreen> {
  final _currentController = TextEditingController();
  final _newController = TextEditingController();
  final _confirmController = TextEditingController();

  bool _submitting = false;
  String? _error;
  bool _success = false;

  @override
  void dispose() {
    _currentController.dispose();
    _newController.dispose();
    _confirmController.dispose();
    super.dispose();
  }

  bool get _isValid {
    if (_currentController.text.length != 6) return false;
    if (_newController.text.length != 6) return false;
    if (_confirmController.text.length != 6) return false;
    if (_newController.text != _confirmController.text) return false;
    return true;
  }

  Future<void> _submit() async {
    if (!_isValid || _submitting) return;

    setState(() {
      _submitting = true;
      _error = null;
    });

    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 700));
      if (!mounted) return;
      setState(() {
        _submitting = false;
        _success = true;
      });
      return;
    }

    try {
      await widget.walletController.setupPin(_newController.text);
      if (!mounted) return;
      setState(() {
        _submitting = false;
        _success = true;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _submitting = false;
        _error = 'Could not change your PIN. Please try again.';
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Scaffold(
      backgroundColor: colors.background,
      body: SafeArea(
        child: Column(
          children: [
            _buildTopBar(context),
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.fromLTRB(24, 16, 24, 24),
                child: _success
                    ? _SuccessView(
                        onDone: () => Navigator.of(context).pop(),
                      )
                    : _buildForm(context),
              ),
            ),
            if (!_success) _buildBottomButton(context),
          ],
        ),
      ),
    );
  }

  // ── Top bar ───────────────────────────────────────────────────────

  Widget _buildTopBar(BuildContext context) {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
      child: Stack(
        alignment: Alignment.center,
        children: [
          Align(
            alignment: Alignment.centerLeft,
            child: GestureDetector(
              onTap: () => Navigator.of(context).pop(),
              behavior: HitTestBehavior.opaque,
              child: Padding(
                padding: const EdgeInsets.all(8),
                child: Icon(
                  Icons.arrow_back_rounded,
                  size: 22,
                  color: colors.text,
                ),
              ),
            ),
          ),
          Text(
            'Change transaction PIN',
            style: TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w700,
              color: colors.text,
              letterSpacing: -0.2,
            ),
          ),
          const SizedBox(width: 40),
        ],
      ),
    );
  }

  // ── Form ──────────────────────────────────────────────────────────

  Widget _buildForm(BuildContext context) {
    final colors = context.palette;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Center(
          child: Container(
            width: 88,
            height: 88,
            decoration: BoxDecoration(
              color: colors.teal10,
              shape: BoxShape.circle,
            ),
            child: Center(
              child: Icon(
                Icons.lock_outline_rounded,
                size: 38,
                color: colors.brandTeal500,
              ),
            ),
          ),
        ),
        const SizedBox(height: 22),
        Text(
          'Update your transaction PIN',
          textAlign: TextAlign.center,
          style: TextStyle(
            fontSize: 20,
            fontWeight: FontWeight.w800,
            color: colors.text,
            letterSpacing: -0.4,
            height: 1.2,
          ),
        ),
        const SizedBox(height: 10),
        Text(
          'Enter your current PIN, then choose a new 6-digit PIN.',
          textAlign: TextAlign.center,
          style: TextStyle(
            fontSize: 14,
            color: colors.textMuted,
            height: 1.5,
          ),
        ),
        const SizedBox(height: 28),

        const _FieldLabel('Current PIN'),
        const SizedBox(height: 10),
        _PinField(
          controller: _currentController,
          onChanged: (_) => setState(() {}),
        ),
        const SizedBox(height: 18),

        const _FieldLabel('New PIN'),
        const SizedBox(height: 10),
        _PinField(
          controller: _newController,
          onChanged: (_) => setState(() {}),
        ),
        const SizedBox(height: 18),

        const _FieldLabel('Confirm new PIN'),
        const SizedBox(height: 10),
        _PinField(
          controller: _confirmController,
          onChanged: (_) => setState(() {}),
        ),

        if (_error != null) ...[
          const SizedBox(height: 16),
          _ErrorBanner(message: _error!),
        ],
      ],
    );
  }

  // ── Bottom button ─────────────────────────────────────────────────

  Widget _buildBottomButton(BuildContext context) {
    return SafeArea(
      top: false,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(24, 8, 24, 16),
        child: SizedBox(
          width: double.infinity,
          height: 56,
          child: FilledButton(
            onPressed: _isValid && !_submitting ? _submit : null,
            style: FilledButton.styleFrom(
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(14),
              ),
              textStyle: const TextStyle(
                fontSize: 16,
                fontWeight: FontWeight.w700,
              ),
            ),
            child: _submitting
                ? const SizedBox(
                    width: 22,
                    height: 22,
                    child: CircularProgressIndicator(
                      color: Colors.white,
                      strokeWidth: 2.2,
                    ),
                  )
                : const Text('Update PIN'),
          ),
        ),
      ),
    );
  }
}

// ── Success view (with animated check) ─────────────────────────────

class _SuccessView extends StatefulWidget {
  const _SuccessView({required this.onDone});
  final VoidCallback onDone;

  @override
  State<_SuccessView> createState() => _SuccessViewState();
}

class _SuccessViewState extends State<_SuccessView>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller;
  late final Animation<double> _circleScale;
  late final Animation<double> _checkProgress;

  @override
  void initState() {
    super.initState();

    _controller = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1100),
    );

    _circleScale = Tween<double>(begin: 0.6, end: 1.0).animate(
      CurvedAnimation(
        parent: _controller,
        curve: const Interval(0.0, 0.42, curve: Curves.easeOutBack),
      ),
    );

    _checkProgress = CurvedAnimation(
      parent: _controller,
      curve: const Interval(0.38, 1.0, curve: Curves.easeOutCubic),
    );

    _controller.forward();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 60),
      child: Column(
        children: [
          AnimatedBuilder(
            animation: _controller,
            builder: (context, _) {
              return Transform.scale(
                scale: _circleScale.value,
                child: SizedBox(
                  width: 96,
                  height: 96,
                  child: CustomPaint(
                    painter: _SuccessCheckPainter(
                      circleColor: colors.success,
                      checkColor: Colors.white,
                      checkProgress: _checkProgress.value,
                    ),
                  ),
                ),
              );
            },
          ),
          const SizedBox(height: 28),
          Text(
            'PIN updated!',
            textAlign: TextAlign.center,
            style: TextStyle(
              fontSize: 22,
              fontWeight: FontWeight.w800,
              color: colors.text,
              letterSpacing: -0.4,
            ),
          ),
          const SizedBox(height: 12),
          Text(
            'Your new transaction PIN is now active.',
            textAlign: TextAlign.center,
            style: TextStyle(
              fontSize: 14,
              color: colors.textMuted,
              height: 1.5,
            ),
          ),
          const SizedBox(height: 40),
          SizedBox(
            width: double.infinity,
            height: 56,
            child: FilledButton(
              onPressed: widget.onDone,
              style: FilledButton.styleFrom(
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(14),
                ),
                textStyle: const TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w700,
                ),
              ),
              child: const Text('Done'),
            ),
          ),
        ],
      ),
    );
  }
}

// ── Success check painter ──────────────────────────────────────────

class _SuccessCheckPainter extends CustomPainter {
  _SuccessCheckPainter({
    required this.circleColor,
    required this.checkColor,
    required this.checkProgress,
  });

  final Color circleColor;
  final Color checkColor;
  final double checkProgress;

  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width / 2, size.height / 2);
    final radius = size.width / 2;

    canvas.drawCircle(center, radius, Paint()..color = circleColor);

    final shortStart = Offset(size.width * 0.30, size.height * 0.52);
    final joint = Offset(size.width * 0.44, size.height * 0.66);
    final tip = Offset(size.width * 0.72, size.height * 0.36);

    final fullPath = Path()
      ..moveTo(shortStart.dx, shortStart.dy)
      ..lineTo(joint.dx, joint.dy)
      ..lineTo(tip.dx, tip.dy);

    final metrics = fullPath.computeMetrics().toList();
    if (metrics.isEmpty || checkProgress <= 0) return;
    final metric = metrics.first;
    final drawLength = metric.length * checkProgress.clamp(0.0, 1.0);
    final partialPath = metric.extractPath(0.0, drawLength);

    final checkPaint = Paint()
      ..color = checkColor
      ..style = PaintingStyle.stroke
      ..strokeWidth = size.width * 0.085
      ..strokeCap = StrokeCap.round
      ..strokeJoin = StrokeJoin.round;

    canvas.drawPath(partialPath, checkPaint);
  }

  @override
  bool shouldRepaint(covariant _SuccessCheckPainter oldDelegate) =>
      oldDelegate.circleColor != circleColor ||
      oldDelegate.checkColor != checkColor ||
      oldDelegate.checkProgress != checkProgress;
}

// ── Field label ─────────────────────────────────────────────────────

class _FieldLabel extends StatelessWidget {
  const _FieldLabel(this.text);
  final String text;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Text(
      text,
      style: TextStyle(
        fontSize: 14,
        fontWeight: FontWeight.w600,
        color: colors.text,
      ),
    );
  }
}

// ── PIN field ───────────────────────────────────────────────────────

class _PinField extends StatefulWidget {
  const _PinField({required this.controller, required this.onChanged});

  final TextEditingController controller;
  final ValueChanged<String> onChanged;

  @override
  State<_PinField> createState() => _PinFieldState();
}

class _PinFieldState extends State<_PinField> {
  bool _obscured = true;
  bool _focused = false;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Focus(
      canRequestFocus: false,
      skipTraversal: true,
      onFocusChange: (focused) => setState(() => _focused = focused),
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 150),
        height: 58,
        decoration: BoxDecoration(
          color: colors.surface,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: _focused ? colors.brandTeal500 : colors.divider,
            width: _focused ? 1.6 : 1,
          ),
        ),
        child: Row(
          children: [
            const SizedBox(width: 16),
            Expanded(
              child: TextField(
                controller: widget.controller,
                keyboardType: TextInputType.number,
                obscureText: _obscured,
                maxLength: 6,
                inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                onChanged: widget.onChanged,
                style: TextStyle(
                  fontSize: 18,
                  fontWeight: FontWeight.w600,
                  color: colors.text,
                  letterSpacing: 6,
                ),
                decoration: InputDecoration(
                  hintText: '••••••',
                  hintStyle: TextStyle(
                    color: colors.textDisabled,
                    letterSpacing: 6,
                    fontWeight: FontWeight.w500,
                    fontSize: 18,
                  ),
                  border: InputBorder.none,
                  counterText: '',
                  isDense: true,
                  contentPadding: const EdgeInsets.symmetric(vertical: 18),
                ),
              ),
            ),
            IconButton(
              onPressed: () => setState(() => _obscured = !_obscured),
              icon: Icon(
                _obscured
                    ? Icons.visibility_off_outlined
                    : Icons.visibility_outlined,
                size: 20,
                color: colors.textMuted,
              ),
            ),
            const SizedBox(width: 6),
          ],
        ),
      ),
    );
  }
}

// ── Error banner ────────────────────────────────────────────────────

class _ErrorBanner extends StatelessWidget {
  const _ErrorBanner({required this.message});
  final String message;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
      decoration: BoxDecoration(
        color: colors.dangerSoft,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: colors.danger, width: 1),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(
            Icons.error_outline_rounded,
            size: 18,
            color: colors.danger,
          ),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              message,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w500,
                color: colors.danger,
                height: 1.4,
              ),
            ),
          ),
        ],
      ),
    );
  }
}