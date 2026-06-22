import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

class CustomerFigmaColors {
  static const primary = Color(0xFF22A84A);
  static const primarySoft = Color(0xFFB8E0C2);
  static const primaryPale = Color(0xFFD7EEDB);
  static const darkGreen = Color(0xFF2F5135);
  static const darkGreenAlt = Color(0xFF2A4830);
  static const text = Color(0xFF121A14);
  static const muted = Color(0xFF7B827C);
  static const surface = Color(0xFFF7F8F7);
  static const field = Color(0xFFFFFFFF);
  static const border = Color(0xFFE5E9E5);
  static const primaryTint = Color(0xFFEAF8EE);

  const CustomerFigmaColors._();
}

class CustomerFigmaAssets {
  static const authCar = 'assets/figma/auth_car_header.png';
  static const locationMap = 'assets/figma/location_map.png';
  static const notificationBell = 'assets/figma/notification_bell.png';
  static const updatesTag = 'assets/figma/updates_tag.png';
  static const allSetPeople = 'assets/figma/all_set_people.png';

  const CustomerFigmaAssets._();
}

class FigmaPhoneScaffold extends StatelessWidget {
  const FigmaPhoneScaffold({
    super.key,
    required this.child,
    this.bottom,
    this.backgroundColor = CustomerFigmaColors.surface,
    this.padding = const EdgeInsets.fromLTRB(24, 18, 24, 16),
  });

  final Widget child;
  final Widget? bottom;
  final Color backgroundColor;
  final EdgeInsetsGeometry padding;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: backgroundColor,
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 430),
            child: Padding(
              padding: padding,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Expanded(child: child),
                  ?bottom,
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class FigmaPrimaryButton extends StatelessWidget {
  const FigmaPrimaryButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.isLoading = false,
  });

  final String label;
  final VoidCallback? onPressed;
  final bool isLoading;

  @override
  Widget build(BuildContext context) {
    final enabled = onPressed != null && !isLoading;

    return SizedBox(
      height: 48,
      child: FilledButton(
        onPressed: enabled ? onPressed : null,
        style: FilledButton.styleFrom(
          backgroundColor: CustomerFigmaColors.primary,
          foregroundColor: Colors.white,
          disabledBackgroundColor: CustomerFigmaColors.primarySoft,
          disabledForegroundColor: Colors.white,
          textStyle: const TextStyle(fontSize: 15, fontWeight: FontWeight.w800),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(999),
          ),
          elevation: 0,
        ),
        child: isLoading
            ? const SizedBox.square(
                dimension: 18,
                child: CircularProgressIndicator(
                  strokeWidth: 2,
                  valueColor: AlwaysStoppedAnimation<Color>(Colors.white),
                ),
              )
            : Text(label),
      ),
    );
  }
}

class FigmaSecondaryButton extends StatelessWidget {
  const FigmaSecondaryButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.dark = false,
  });

  final String label;
  final VoidCallback onPressed;
  final bool dark;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 46,
      child: FilledButton(
        onPressed: onPressed,
        style: FilledButton.styleFrom(
          backgroundColor: dark
              ? const Color(0xFF356D42)
              : CustomerFigmaColors.primaryPale,
          foregroundColor: dark ? Colors.white : CustomerFigmaColors.primary,
          textStyle: const TextStyle(fontSize: 14, fontWeight: FontWeight.w800),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(999),
          ),
          elevation: 0,
        ),
        child: Text(label, textAlign: TextAlign.center),
      ),
    );
  }
}

class FigmaBackButton extends StatelessWidget {
  const FigmaBackButton({super.key, required this.onPressed});

  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onPressed,
      child: SizedBox(
        height: 32,
        child: const Icon(Icons.arrow_back_rounded),
      ),
    );
  }
}

class FigmaProgressHeader extends StatelessWidget {
  const FigmaProgressHeader({super.key, required this.progress, this.onBack});

  final double progress;
  final VoidCallback? onBack;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        if (onBack != null) ...[
          GestureDetector(
            onTap: onBack,
            child: const Icon(Icons.arrow_back_rounded),
          ),
          const SizedBox(width: 4),
        ],
        Expanded(
          child: ClipRRect(
            borderRadius: BorderRadius.circular(99),
            child: LinearProgressIndicator(
              value: progress,
              minHeight: 7,
              backgroundColor: CustomerFigmaColors.primaryPale,
              color: CustomerFigmaColors.primary,
            ),
          ),
        ),
      ],
    );
  }
}

class FigmaTextField extends StatelessWidget {
  const FigmaTextField({
    super.key,
    required this.controller,
    required this.label,
    this.hintText,
    this.keyboardType,
    this.readOnly = false,
    this.inputFormatters,
    this.onSubmitted,
  });

  final TextEditingController controller;
  final String label;
  final String? hintText;
  final TextInputType? keyboardType;
  final bool readOnly;
  final List<TextInputFormatter>? inputFormatters;
  final ValueChanged<String>? onSubmitted;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: const TextStyle(
            color: CustomerFigmaColors.text,
            fontSize: 13,
            fontWeight: FontWeight.w800,
          ),
        ),
        const SizedBox(height: 8),
        TextField(
          controller: controller,
          keyboardType: keyboardType,
          readOnly: readOnly,
          inputFormatters: inputFormatters,
          textInputAction: TextInputAction.next,
          onSubmitted: onSubmitted,
          decoration: InputDecoration(
            hintText: hintText,
            filled: true,
            fillColor: CustomerFigmaColors.field,
            contentPadding: const EdgeInsets.symmetric(
              horizontal: 16,
              vertical: 14,
            ),
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(10),
              borderSide: BorderSide.none,
            ),
            enabledBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(10),
              borderSide: BorderSide.none,
            ),
            focusedBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(10),
              borderSide: const BorderSide(color: CustomerFigmaColors.primary),
            ),
          ),
        ),
      ],
    );
  }
}

class FigmaCheckeredCircle extends StatelessWidget {
  const FigmaCheckeredCircle({super.key});

  @override
  Widget build(BuildContext context) {
    return ClipOval(
      child: CustomPaint(
        painter: _CheckerPainter(),
        child: const SizedBox(width: 230, height: 230),
      ),
    );
  }
}

class _CheckerPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    const tile = 18.0;
    final light = Paint()..color = const Color(0xFFF8F8F8);
    final mid = Paint()..color = const Color(0xFFEFEFEF);
    canvas.drawRect(Offset.zero & size, light);
    for (var y = 0.0; y < size.height; y += tile) {
      for (var x = 0.0; x < size.width; x += tile) {
        final index = ((x / tile).floor() + (y / tile).floor()) % 2;
        if (index == 0) {
          canvas.drawRect(Rect.fromLTWH(x, y, tile, tile), mid);
        }
      }
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
