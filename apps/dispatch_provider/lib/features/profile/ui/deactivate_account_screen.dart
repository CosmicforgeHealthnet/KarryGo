import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../home/ui/widgets/home_feedback.dart';
import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../state/provider_profile_controller.dart';

class DeactivateAccountScreen extends StatefulWidget {
  const DeactivateAccountScreen({
    super.key,
    required this.profileController,
    this.onDeleted,
  });

  final ProviderProfileController profileController;
  final VoidCallback? onDeleted;

  @override
  State<DeactivateAccountScreen> createState() =>
      _DeactivateAccountScreenState();
}

class _DeactivateAccountScreenState extends State<DeactivateAccountScreen> {
  bool _isSubmitting = false;

  Future<void> _confirm() async {
    if (_isSubmitting) return;
    final confirmed = await confirmAccountAction(
      context,
      title: 'Deactivate your account?',
      message: 'Your account will be deactivated and you will be signed out.',
      action: 'Deactivate account',
      symbol: JourneySymbol.profile,
      destructive: true,
    );
    if (!confirmed || !mounted) return;
    setState(() => _isSubmitting = true);

    final result = await widget.profileController.deleteAccount();

    if (!mounted) return;
    setState(() => _isSubmitting = false);

    String? error;
    result.when(success: (_) {}, failure: (e) => error = e.message);

    if (error != null) {
      await showHomeFeedback(
        context,
        title: 'Could not deactivate account',
        message: error!,
        error: true,
      );
      return;
    }

    Navigator.of(context).pop();
    widget.onDeleted?.call();
  }

  @override
  Widget build(BuildContext context) => AccountPage(
    title: 'Deactivate account',
    symbol: JourneySymbol.profile,
    description: 'Deactivation removes your access to this rider account.',
    bottom: FilledButton(
      onPressed: _isSubmitting ? null : _confirm,
      style: FilledButton.styleFrom(
        backgroundColor: context.palette.danger,
        foregroundColor: Colors.white,
      ),
      child: _isSubmitting
          ? const SizedBox.square(
              dimension: 20,
              child: CircularProgressIndicator(strokeWidth: 2),
            )
          : const Text('Deactivate account'),
    ),
    children: [
      AccountSection(
        title: 'Before you continue',
        children: [
          AccountMenuTile(
            title: 'Check your wallet',
            subtitle: 'Review any available funds and pending withdrawals.',
            symbol: JourneySymbol.wallet,
          ),
          AccountMenuTile(
            title: 'Account access will end',
            subtitle: 'You will be signed out after deactivation is confirmed.',
            symbol: JourneySymbol.signOut,
            color: context.palette.danger,
          ),
        ],
      ),
      TextButton(
        onPressed: _isSubmitting ? null : () => Navigator.of(context).pop(),
        child: const Text('Keep my account'),
      ),
    ],
  );
}
