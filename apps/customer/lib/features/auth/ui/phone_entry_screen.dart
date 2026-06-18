import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../app/app_routes.dart';
import '../../../shared/widgets/figma_customer_widgets.dart';
import '../state/customer_auth_controller.dart';

class PhoneEntryScreen extends StatefulWidget {
  const PhoneEntryScreen({
    super.key,
    required this.controller,
    required this.state,
  });

  final CustomerAuthController controller;
  final CustomerAuthState state;

  @override
  State<PhoneEntryScreen> createState() => _PhoneEntryScreenState();
}

class _PhoneEntryScreenState extends State<PhoneEntryScreen> {
  late final TextEditingController _phoneController;
  late final TextEditingController _emailController;
  late CustomerAuthIdentifierType _identifierType;

  @override
  void initState() {
    super.initState();
    _identifierType = widget.state.identifierType;
    _phoneController = TextEditingController(
      text: _displayPhone(widget.state.phone),
    );
    _emailController = TextEditingController(text: widget.state.email);
  }

  @override
  void dispose() {
    _phoneController.dispose();
    _emailController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final field = _identifierType == CustomerAuthIdentifierType.email
        ? 'email'
        : 'phone';
    final fieldError =
        _fieldError(widget.state.error, field) ??
        _fieldError(widget.state.error, 'identifier');
    final activeController = _identifierType == CustomerAuthIdentifierType.email
        ? _emailController
        : _phoneController;

    return FigmaPhoneScaffold(
      key: const ValueKey(CustomerAppRoutes.phoneEntry),
      child: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const SizedBox(height: 18),
            Image.asset(CustomerFigmaAssets.authCar, height: 92),
            const SizedBox(height: 24),
            const Text(
              'Welcome to Cosmicforge Logistics!',
              style: TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 17,
                fontWeight: FontWeight.w900,
              ),
            ),
            const SizedBox(height: 4),
            const Text(
              "Let's get you moving.",
              style: TextStyle(
                color: CustomerFigmaColors.primary,
                fontSize: 13,
                fontWeight: FontWeight.w800,
              ),
            ),
            const SizedBox(height: 18),
            const Text(
              'Enter your phone number or email to continue.',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 12),
            ),
            const SizedBox(height: 24),
            _IdentifierSegmentedControl(
              value: _identifierType,
              onChanged: (value) {
                setState(() {
                  _identifierType = value;
                });
              },
            ),
            const SizedBox(height: 20),
            const Text(
              'Enter your details',
              style: TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 13,
                fontWeight: FontWeight.w900,
              ),
            ),
            const SizedBox(height: 8),
            if (_identifierType == CustomerAuthIdentifierType.email)
              _EmailField(controller: _emailController)
            else
              _PhoneNumberField(controller: _phoneController),
            if (fieldError != null) ...[
              const SizedBox(height: 8),
              CosmicforgeLogisticsFieldError(message: fieldError),
            ] else ...[
              const SizedBox(height: 8),
              const Text(
                "We'll send you a verification code.",
                style: TextStyle(
                  color: CustomerFigmaColors.primary,
                  fontSize: 12,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ],
            if (widget.state.error != null && fieldError == null) ...[
              const SizedBox(height: 16),
              CosmicforgeLogisticsErrorBanner(
                title: widget.state.error!.title,
                message: widget.state.error!.message,
                onClose: widget.controller.dismissError,
              ),
            ],
            const SizedBox(height: 34),
            ValueListenableBuilder<TextEditingValue>(
              valueListenable: activeController,
              builder: (context, value, _) {
                final canContinue = value.text.trim().isNotEmpty;
                return FigmaPrimaryButton(
                  label: 'Continue',
                  isLoading: widget.state.isLoading,
                  onPressed: canContinue && !widget.state.isLoading
                      ? _continue
                      : null,
                );
              },
            ),
            const SizedBox(height: 28),
            const _DividerLabel(label: 'Or'),
            const SizedBox(height: 20),
            _SocialButton(
              label: 'Continue with Google',
              iconText: 'G',
              onPressed: _showSocialUnavailable,
            ),
            const SizedBox(height: 12),
            _SocialButton(
              label: 'Continue with Apple',
              icon: Icons.apple,
              dark: true,
              onPressed: _showSocialUnavailable,
            ),
            const SizedBox(height: 26),
            Text.rich(
              TextSpan(
                text: 'Already have an account? ',
                style: const TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 12,
                ),
                children: const [
                  TextSpan(
                    text: 'Log In',
                    style: TextStyle(
                      color: CustomerFigmaColors.primary,
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                ],
              ),
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    );
  }

  void _continue() {
    FocusScope.of(context).unfocus();
    widget.controller.startAuth(
      type: _identifierType,
      value: _identifierType == CustomerAuthIdentifierType.email
          ? _emailController.text
          : _normalizedPhone(_phoneController.text),
    );
  }

  void _showSocialUnavailable() {
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(
        content: Text('Phone authentication is available for now.'),
      ),
    );
  }
}

class _IdentifierSegmentedControl extends StatelessWidget {
  const _IdentifierSegmentedControl({
    required this.value,
    required this.onChanged,
  });

  final CustomerAuthIdentifierType value;
  final ValueChanged<CustomerAuthIdentifierType> onChanged;

  @override
  Widget build(BuildContext context) {
    return SegmentedButton<CustomerAuthIdentifierType>(
      segments: const [
        ButtonSegment(
          value: CustomerAuthIdentifierType.phone,
          label: Text('Phone'),
          icon: Icon(Icons.phone_iphone_rounded),
        ),
        ButtonSegment(
          value: CustomerAuthIdentifierType.email,
          label: Text('Email'),
          icon: Icon(Icons.alternate_email_rounded),
        ),
      ],
      selected: {value},
      onSelectionChanged: (selection) => onChanged(selection.single),
      style: ButtonStyle(
        visualDensity: VisualDensity.compact,
        textStyle: WidgetStateProperty.all(
          const TextStyle(fontSize: 12, fontWeight: FontWeight.w800),
        ),
        foregroundColor: WidgetStateProperty.resolveWith(
          (states) => states.contains(WidgetState.selected)
              ? Colors.white
              : CustomerFigmaColors.text,
        ),
        backgroundColor: WidgetStateProperty.resolveWith(
          (states) => states.contains(WidgetState.selected)
              ? CustomerFigmaColors.primary
              : Colors.white,
        ),
        side: WidgetStateProperty.all(
          const BorderSide(color: CustomerFigmaColors.primary),
        ),
      ),
    );
  }
}

class _PhoneNumberField extends StatelessWidget {
  const _PhoneNumberField({required this.controller});

  final TextEditingController controller;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Container(
          height: 48,
          padding: const EdgeInsets.symmetric(horizontal: 12),
          decoration: BoxDecoration(
            color: Colors.white,
            borderRadius: BorderRadius.circular(8),
            border: Border.all(color: CustomerFigmaColors.primary),
          ),
          child: const Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              _NigeriaFlagMark(),
              SizedBox(width: 8),
              Text(
                '+234',
                style: TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 12,
                  fontWeight: FontWeight.w700,
                ),
              ),
              Icon(Icons.keyboard_arrow_down_rounded, size: 16),
            ],
          ),
        ),
        const SizedBox(width: 8),
        Expanded(
          child: TextField(
            controller: controller,
            keyboardType: TextInputType.phone,
            textInputAction: TextInputAction.done,
            autofillHints: const [AutofillHints.telephoneNumber],
            inputFormatters: [
              FilteringTextInputFormatter.allow(RegExp(r'[0-9+ ]')),
            ],
            decoration: InputDecoration(
              hintText: '8067735987',
              filled: true,
              fillColor: Colors.white,
              contentPadding: const EdgeInsets.symmetric(
                horizontal: 14,
                vertical: 14,
              ),
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(8),
                borderSide: const BorderSide(
                  color: CustomerFigmaColors.primary,
                ),
              ),
              enabledBorder: OutlineInputBorder(
                borderRadius: BorderRadius.circular(8),
                borderSide: const BorderSide(
                  color: CustomerFigmaColors.primary,
                ),
              ),
              focusedBorder: OutlineInputBorder(
                borderRadius: BorderRadius.circular(8),
                borderSide: const BorderSide(
                  color: CustomerFigmaColors.primary,
                ),
              ),
            ),
            onSubmitted: (_) {},
          ),
        ),
      ],
    );
  }
}

class _EmailField extends StatelessWidget {
  const _EmailField({required this.controller});

  final TextEditingController controller;

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      keyboardType: TextInputType.emailAddress,
      textInputAction: TextInputAction.done,
      autofillHints: const [AutofillHints.email],
      decoration: InputDecoration(
        hintText: 'ada@example.com',
        filled: true,
        fillColor: Colors.white,
        contentPadding: const EdgeInsets.symmetric(
          horizontal: 14,
          vertical: 14,
        ),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: const BorderSide(color: CustomerFigmaColors.primary),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: const BorderSide(color: CustomerFigmaColors.primary),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: const BorderSide(color: CustomerFigmaColors.primary),
        ),
      ),
      onSubmitted: (_) {},
    );
  }
}

class _NigeriaFlagMark extends StatelessWidget {
  const _NigeriaFlagMark();

  @override
  Widget build(BuildContext context) {
    return ClipRRect(
      borderRadius: BorderRadius.circular(2),
      child: Row(
        children: const [
          _FlagStripe(color: CustomerFigmaColors.primary),
          _FlagStripe(color: Colors.white),
          _FlagStripe(color: CustomerFigmaColors.primary),
        ],
      ),
    );
  }
}

class _FlagStripe extends StatelessWidget {
  const _FlagStripe({required this.color});

  final Color color;

  @override
  Widget build(BuildContext context) {
    return Container(width: 8, height: 18, color: color);
  }
}

class _DividerLabel extends StatelessWidget {
  const _DividerLabel({required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        const Expanded(child: Divider(color: CustomerFigmaColors.border)),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12),
          child: Text(
            label,
            style: const TextStyle(
              color: CustomerFigmaColors.primarySoft,
              fontWeight: FontWeight.w700,
            ),
          ),
        ),
        const Expanded(child: Divider(color: CustomerFigmaColors.border)),
      ],
    );
  }
}

class _SocialButton extends StatelessWidget {
  const _SocialButton({
    required this.label,
    required this.onPressed,
    this.icon,
    this.iconText,
    this.dark = false,
  });

  final String label;
  final VoidCallback onPressed;
  final IconData? icon;
  final String? iconText;
  final bool dark;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 48,
      child: FilledButton(
        onPressed: onPressed,
        style: FilledButton.styleFrom(
          backgroundColor: dark ? Colors.black : Colors.white,
          foregroundColor: dark ? Colors.white : CustomerFigmaColors.text,
          elevation: dark ? 0 : 8,
          shadowColor: Colors.black.withValues(alpha: 0.08),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(999),
            side: dark
                ? BorderSide.none
                : const BorderSide(color: CustomerFigmaColors.border),
          ),
          textStyle: const TextStyle(fontSize: 15, fontWeight: FontWeight.w700),
        ),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            if (icon != null)
              Icon(icon, size: 20)
            else
              Text(
                iconText ?? '',
                style: TextStyle(
                  color: dark ? Colors.white : Colors.blue,
                  fontSize: 18,
                  fontWeight: FontWeight.w900,
                ),
              ),
            const SizedBox(width: 12),
            Text(label),
          ],
        ),
      ),
    );
  }
}

String _displayPhone(String phone) {
  if (phone.startsWith('+234')) {
    return phone.substring(4);
  }
  return phone;
}

String _normalizedPhone(String phone) {
  final value = phone.replaceAll(' ', '').trim();
  if (value.startsWith('+')) {
    return value;
  }
  if (value.startsWith('0')) {
    return '+234${value.substring(1)}';
  }
  return '+234$value';
}

String? _fieldError(ApiException? error, String field) {
  if (error == null) {
    return null;
  }
  for (final fieldError in error.fields) {
    if (fieldError.field == field) {
      return fieldError.message;
    }
  }
  return null;
}
