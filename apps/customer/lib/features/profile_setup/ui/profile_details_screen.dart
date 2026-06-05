import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../app/app_routes.dart';
import '../../../shared/widgets/figma_customer_widgets.dart';
import '../../auth/state/customer_auth_controller.dart';

class ProfileDetailsScreen extends StatefulWidget {
  const ProfileDetailsScreen({
    super.key,
    required this.controller,
    required this.state,
  });

  final CustomerAuthController controller;
  final CustomerAuthState state;

  @override
  State<ProfileDetailsScreen> createState() => _ProfileDetailsScreenState();
}

class _ProfileDetailsScreenState extends State<ProfileDetailsScreen> {
  late final TextEditingController _nameController;
  late final TextEditingController _phoneController;
  late final TextEditingController _emailController;

  @override
  void initState() {
    super.initState();
    _nameController = TextEditingController(text: widget.state.profileName);
    _phoneController = TextEditingController(text: widget.state.phone);
    _emailController = TextEditingController(text: widget.state.profileEmail);
  }

  @override
  void dispose() {
    _nameController.dispose();
    _phoneController.dispose();
    _emailController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return FigmaPhoneScaffold(
      key: const ValueKey(CustomerAppRoutes.profileDetails),
      bottom: _ProfileDetailsButton(
        nameController: _nameController,
        emailController: _emailController,
        onContinue: _continue,
      ),
      child: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            FigmaProgressHeader(
              progress: 0.5,
              onBack: widget.controller.goToServiceChoice,
            ),
            const SizedBox(height: 34),
            const Text(
              'Tell us about you',
              style: TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 18,
                fontWeight: FontWeight.w900,
              ),
            ),
            const SizedBox(height: 12),
            const Text(
              'Set up your account so you can start booking rides, send packages and more.',
              style: TextStyle(
                color: CustomerFigmaColors.muted,
                fontSize: 12,
                height: 1.45,
              ),
            ),
            const SizedBox(height: 30),
            FigmaTextField(
              controller: _nameController,
              label: 'Your Full Name',
              hintText: 'Demaunix_1',
              onSubmitted: (_) => _continue(),
            ),
            const SizedBox(height: 20),
            FigmaTextField(
              controller: _phoneController,
              label: 'Your Phone Number',
              keyboardType: TextInputType.phone,
              readOnly: true,
              inputFormatters: [
                FilteringTextInputFormatter.allow(RegExp(r'[0-9+ ]')),
              ],
            ),
            const SizedBox(height: 20),
            FigmaTextField(
              controller: _emailController,
              label: 'Email',
              hintText: 'Demaunix_1',
              keyboardType: TextInputType.emailAddress,
              onSubmitted: (_) => _continue(),
            ),
          ],
        ),
      ),
    );
  }

  void _continue() {
    final fullName = _nameController.text.trim();
    final email = _emailController.text.trim();
    if (fullName.isEmpty || email.isEmpty) {
      return;
    }
    widget.controller.completeProfileDetails(fullName: fullName, email: email);
  }
}

class _ProfileDetailsButton extends StatelessWidget {
  const _ProfileDetailsButton({
    required this.nameController,
    required this.emailController,
    required this.onContinue,
  });

  final TextEditingController nameController;
  final TextEditingController emailController;
  final VoidCallback onContinue;

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: Listenable.merge([nameController, emailController]),
      builder: (context, _) {
        final enabled =
            nameController.text.trim().isNotEmpty &&
            emailController.text.trim().isNotEmpty;
        return FigmaPrimaryButton(
          label: 'Continue',
          onPressed: enabled ? onContinue : null,
        );
      },
    );
  }
}
