import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../../auth/models/customer_auth_models.dart';
import '../../../support/data/support_api.dart';
import '../../models/wallet_models.dart';
import '../widgets/wallet_flow_scaffold.dart';
import '../widgets/wallet_transaction_tile.dart';

/// Log Disputes flow for a wallet transaction (mockup #3-#5).
///
/// Entered from a transaction, so the transaction is already selected. The user
/// picks a dispute type and a note, then we file a complaint via the existing
/// support/disputes service (`serviceType: 'wallet'`,
/// `bookingReference: <txn reference>`).
class WalletDisputeFlowScreen extends StatefulWidget {
  const WalletDisputeFlowScreen({
    super.key,
    required this.session,
    required this.supportApi,
    required this.transaction,
  });

  final CustomerSession session;
  final SupportApi supportApi;
  final WalletTransaction transaction;

  @override
  State<WalletDisputeFlowScreen> createState() =>
      _WalletDisputeFlowScreenState();
}

class _WalletDisputeFlowScreenState extends State<WalletDisputeFlowScreen> {
  static const _disputeTypes = [
    'successful withdrawal  not credited to bank',
    'Cancelled trip',
    'over deduction',
    'Pending transaction',
    'Failed transaction',
  ];

  String? _selectedType;
  final _notesCtrl = TextEditingController();
  bool _submitting = false;
  ApiException? _error;

  @override
  void dispose() {
    _notesCtrl.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final type = _selectedType;
    if (type == null) {
      setState(() {
        _error = const ApiException(
          code: ApiErrorCode.validationFailed,
          message: 'Please select a dispute type.',
          fields: [],
        );
      });
      return;
    }

    setState(() {
      _submitting = true;
      _error = null;
    });

    try {
      await widget.supportApi.createComplaint(
        accessToken: widget.session.accessToken,
        serviceType: 'wallet',
        subject: type,
        description: _notesCtrl.text.trim().isEmpty
            ? 'Dispute for transaction ${widget.transaction.reference}: $type'
            : _notesCtrl.text.trim(),
        bookingReference: widget.transaction.reference,
      );
      if (mounted) {
        _showSuccess();
      }
    } on ApiException catch (e) {
      setState(() {
        _error = e;
        _submitting = false;
      });
    }
  }

  void _showSuccess() {
    showModalBottomSheet<void>(
      context: context,
      isDismissible: false,
      backgroundColor: Colors.white,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
      ),
      builder: (sheetContext) => Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 64,
              height: 64,
              decoration: const BoxDecoration(
                color: CustomerFigmaColors.primary,
                shape: BoxShape.circle,
              ),
              child: const Icon(Icons.check_rounded,
                  color: Colors.white, size: 32),
            ),
            const SizedBox(height: 16),
            const Text(
              'Dispute Submitted',
              style: TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 18,
                fontWeight: FontWeight.w900,
              ),
            ),
            const SizedBox(height: 8),
            const Text(
              'Our support team will review your dispute and get back to you.',
              textAlign: TextAlign.center,
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
            ),
            const SizedBox(height: 20),
            FigmaPrimaryButton(
              label: 'Done',
              onPressed: () {
                Navigator.of(sheetContext).pop();
                Navigator.of(context).pop();
              },
            ),
          ],
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return WalletFlowScaffold(
      title: 'Log Disputes',
      body: ListView(
        padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
        children: [
          const WalletSectionLabel('Select a Transaction'),
          const SizedBox(height: 12),
          WalletTransactionTile(txn: widget.transaction),
          const SizedBox(height: 20),
          const WalletSectionLabel('Select Dispute Type'),
          const SizedBox(height: 12),
          ..._disputeTypes.map(
            (type) => Padding(
              padding: const EdgeInsets.only(bottom: 10),
              child: _DisputeTypeTile(
                label: type,
                selected: _selectedType == type,
                onTap: () => setState(() => _selectedType = type),
              ),
            ),
          ),
          const SizedBox(height: 8),
          _NotesField(controller: _notesCtrl),
          if (_error != null) ...[
            const SizedBox(height: 16),
            _ErrorBanner(message: _error!.message),
          ],
        ],
      ),
      bottom: FigmaPrimaryButton(
        label: 'Confirm',
        isLoading: _submitting,
        onPressed: _submitting ? null : _submit,
      ),
    );
  }
}

class _DisputeTypeTile extends StatelessWidget {
  const _DisputeTypeTile({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: selected
                ? CustomerFigmaColors.primary
                : CustomerFigmaColors.border,
            width: selected ? 1.6 : 1,
          ),
        ),
        child: Row(
          children: [
            Expanded(
              child: Text(
                label,
                style: const TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ),
            Icon(
              selected
                  ? Icons.radio_button_checked_rounded
                  : Icons.radio_button_unchecked_rounded,
              color: selected
                  ? CustomerFigmaColors.primary
                  : CustomerFigmaColors.border,
              size: 22,
            ),
          ],
        ),
      ),
    );
  }
}

class _NotesField extends StatelessWidget {
  const _NotesField({required this.controller});
  final TextEditingController controller;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text(
          'Additional details (optional)',
          style: TextStyle(
            color: CustomerFigmaColors.text,
            fontSize: 13,
            fontWeight: FontWeight.w800,
          ),
        ),
        const SizedBox(height: 8),
        TextField(
          controller: controller,
          maxLines: 4,
          decoration: InputDecoration(
            hintText: 'Tell us more about the issue…',
            filled: true,
            fillColor: CustomerFigmaColors.field,
            contentPadding: const EdgeInsets.all(16),
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

class _ErrorBanner extends StatelessWidget {
  const _ErrorBanner({required this.message});
  final String message;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: const Color(0xFFFFF1F0),
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: const Color(0xFFFFCDD2)),
      ),
      child: Text(
        message,
        style: const TextStyle(color: Color(0xFFC0392B), fontSize: 13),
      ),
    );
  }
}
