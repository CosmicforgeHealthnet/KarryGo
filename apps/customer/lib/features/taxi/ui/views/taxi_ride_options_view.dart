import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../../wallet/ui/funding/wallet_funding_flow_screen.dart';
import '../../state/taxi_booking_controller.dart';
import '../widgets/taxi_flow_helpers.dart';

/// A note for the driver plus the wallet summary — the last screen before the
/// ride is requested.
class TaxiRideOptionsView extends StatefulWidget {
  const TaxiRideOptionsView({
    super.key,
    required this.controller,
    required this.state,
  });

  final TaxiBookingController controller;
  final TaxiBookingState state;

  @override
  State<TaxiRideOptionsView> createState() => _TaxiRideOptionsViewState();
}

class _TaxiRideOptionsViewState extends State<TaxiRideOptionsView> {
  final _notesCtrl = TextEditingController();

  Future<void> _openTopUp() async {
    final token = widget.controller.accessTokenForWallet;
    if (token == null) return;
    final funded = await Navigator.of(context).push<bool>(
      MaterialPageRoute<bool>(
        builder: (_) => WalletFundingFlowScreen(
          walletApi: widget.controller.walletApi,
          accessToken: token,
          customerEmail: widget.controller.customerEmail,
        ),
      ),
    );
    if (funded == true) {
      await widget.controller.refreshWalletBalance();
    }
  }

  @override
  void initState() {
    super.initState();
    _notesCtrl.text = widget.state.rideNotes;
  }

  @override
  void dispose() {
    _notesCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.state;
    final controller = widget.controller;
    final tile = state.selectedClass;
    final fareKobo = tile?.fareKobo ?? 0;
    // A null balance means "not loaded yet" and must never block: the
    // authoritative check is the wallet hold the service performs when a
    // driver accepts.
    final balanceKobo = state.walletBalanceKobo;
    final canAfford = balanceKobo == null || balanceKobo >= fareKobo;

    final isParcel = state.rideMode.isParcel;

    return taxiFlowScaffold(
      title: isParcel ? 'Confirm your delivery' : 'Confirm your ride',
      // A parcel came through the package-details screen; a ride did not.
      onBack: isParcel
          ? controller.backToParcelDetails
          : controller.backToFareSelection,
      body: ListView(
        children: [
          TaxiRouteSummary(
            pickupAddress: state.pickupAddress,
            dropoffAddress: state.dropoffAddress,
          ),
          const SizedBox(height: 18),

          if (tile != null) ...[
            Container(
              padding: const EdgeInsets.all(14),
              decoration: BoxDecoration(
                color: CustomerFigmaColors.primaryTint,
                borderRadius: BorderRadius.circular(14),
              ),
              child: Row(
                children: [
                  const Icon(
                    Icons.local_taxi_rounded,
                    color: CustomerFigmaColors.primary,
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Text(
                      tile.label,
                      style: const TextStyle(
                        color: CustomerFigmaColors.text,
                        fontSize: 15,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                  ),
                  Text(
                    formatHaulingFare(tile.fareKobo),
                    style: const TextStyle(
                      color: CustomerFigmaColors.text,
                      fontSize: 16,
                      fontWeight: FontWeight.w900,
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 18),
          ],

          if (isParcel) ...[
            taxiSectionLabel('Package'),
            _ParcelSummary(state: state, onEdit: controller.backToParcelDetails),
            const SizedBox(height: 18),
          ],

          taxiSectionLabel('Note for the driver (optional)'),
          FigmaTextField(
            controller: _notesCtrl,
            label: '',
            hintText: 'e.g. I am by the blue gate',
            variant: FigmaTextFieldVariant.grey,
            onSubmitted: controller.setRideNotes,
          ),
          const SizedBox(height: 18),

          taxiSectionLabel('Payment'),
          _WalletSummary(
            balanceKobo: state.walletBalanceKobo,
            fareKobo: fareKobo,
            onTopUp: _openTopUp,
          ),

          if (state.error != null) ...[
            const SizedBox(height: 14),
            Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: const Color(0xFFFDECEC),
                borderRadius: BorderRadius.circular(10),
                border: Border.all(color: const Color(0xFFF5B5B5)),
              ),
              child: Text(
                state.error!,
                style: const TextStyle(color: Color(0xFFB3261E), fontSize: 12),
              ),
            ),
          ],
          const SizedBox(height: 10),
        ],
      ),
      bottom: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (!canAfford)
            const Padding(
              padding: EdgeInsets.only(bottom: 8),
              child: Text(
                'Top up your wallet to request this ride.',
                textAlign: TextAlign.center,
                style: TextStyle(color: Color(0xFFB3261E), fontSize: 12),
              ),
            ),
          FigmaPrimaryButton(
            label: isParcel ? 'Request a driver' : 'Request ride',
            isLoading: state.isLoading,
            onPressed: (state.isLoading || !canAfford)
                ? null
                : () {
                    // Commit any note the passenger typed but never submitted.
                    controller.setRideNotes(_notesCtrl.text);
                    controller.confirmBooking();
                  },
          ),
        ],
      ),
    );
  }
}

/// A read-only recap of what the customer entered on the package screen,
/// with a route back to change it.
class _ParcelSummary extends StatelessWidget {
  const _ParcelSummary({required this.state, required this.onEdit});

  final TaxiBookingState state;
  final VoidCallback onEdit;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: CustomerFigmaColors.field,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: CustomerFigmaColors.border),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  state.packageDesc,
                  style: const TextStyle(
                    color: CustomerFigmaColors.text,
                    fontSize: 14,
                    fontWeight: FontWeight.w800,
                  ),
                ),
              ),
              TextButton(
                onPressed: onEdit,
                style: TextButton.styleFrom(
                  padding: EdgeInsets.zero,
                  minimumSize: const Size(0, 28),
                  tapTargetSize: MaterialTapTargetSize.shrinkWrap,
                ),
                child: const Text(
                  'Edit',
                  style: TextStyle(
                    color: CustomerFigmaColors.primary,
                    fontSize: 13,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            'To ${state.receiverName} · ${state.receiverPhone}',
            style: const TextStyle(
              color: CustomerFigmaColors.muted,
              fontSize: 12,
            ),
          ),
          if (state.isFragile) ...[
            const SizedBox(height: 4),
            const Text(
              'Marked fragile',
              style: TextStyle(color: Color(0xFFE9A13B), fontSize: 11.5),
            ),
          ],
        ],
      ),
    );
  }
}

/// Wallet balance versus this fare, with a top-up route when it falls short.
class _WalletSummary extends StatelessWidget {
  const _WalletSummary({
    required this.balanceKobo,
    required this.fareKobo,
    required this.onTopUp,
  });

  final int? balanceKobo;
  final int fareKobo;
  final Future<void> Function() onTopUp;

  @override
  Widget build(BuildContext context) {
    final loaded = balanceKobo != null;
    final short = loaded && balanceKobo! < fareKobo;

    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: CustomerFigmaColors.field,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: short ? const Color(0xFFF5B5B5) : CustomerFigmaColors.border,
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const Icon(
                Icons.account_balance_wallet_rounded,
                size: 20,
                color: CustomerFigmaColors.primary,
              ),
              const SizedBox(width: 12),
              const Expanded(
                child: Text(
                  'Wallet',
                  style: TextStyle(
                    color: CustomerFigmaColors.text,
                    fontSize: 14,
                    fontWeight: FontWeight.w800,
                  ),
                ),
              ),
              Text(
                loaded ? formatHaulingFare(balanceKobo!) : '--',
                style: TextStyle(
                  color: short ? const Color(0xFFB3261E) : CustomerFigmaColors.text,
                  fontSize: 14,
                  fontWeight: FontWeight.w800,
                ),
              ),
            ],
          ),
          const SizedBox(height: 6),
          Text(
            short
                ? 'Not enough for this ride.'
                : 'Held when a driver accepts, released when you arrive.',
            style: TextStyle(
              color: short ? const Color(0xFFB3261E) : CustomerFigmaColors.muted,
              fontSize: 11.5,
            ),
          ),
          const SizedBox(height: 6),
          Align(
            alignment: Alignment.centerLeft,
            child: TextButton(
              onPressed: () => onTopUp(),
              style: TextButton.styleFrom(
                padding: EdgeInsets.zero,
                minimumSize: const Size(0, 32),
                tapTargetSize: MaterialTapTargetSize.shrinkWrap,
              ),
              child: const Text(
                'Top up wallet',
                style: TextStyle(
                  color: CustomerFigmaColors.primary,
                  fontSize: 13,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
