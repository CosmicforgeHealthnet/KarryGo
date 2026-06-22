import 'package:flutter/material.dart';

import '../../../auth/models/provider_auth_models.dart';
import '../../../auth/state/provider_auth_controller.dart';
import '../../state/provider_home_controller.dart';
import '../screens/provider_request_detail_screen.dart';
import '../widgets/provider_app_colors.dart';
import '../widgets/provider_home_map.dart';
import '../widgets/provider_request_card.dart';

/// Map-based home tab (Figma 2034 / 2035).
class ProviderDashboardScreen extends StatelessWidget {
  const ProviderDashboardScreen({
    super.key,
    required this.authController,
    required this.homeController,
    required this.state,
    this.onNotificationsTap,
  });

  final ProviderAuthController authController;
  final ProviderHomeController homeController;
  final ProviderHomeState state;
  final VoidCallback? onNotificationsTap;

  @override
  Widget build(BuildContext context) {
    final topPadding = MediaQuery.of(context).padding.top;

    return Stack(
      children: [
        // ─── Full-screen live map ─────────────────────────────────────────
        const Positioned.fill(child: ProviderHomeMap()),

        // ─── Top overlay: Go Offline/Online pill + notification bell ──────
        Positioned(
          top: topPadding + 12,
          left: 16,
          right: 16,
          child: Row(
            children: [
              _OnlineTogglePill(
                isOnline: state.isOnline,
                isLoading: state.isLoading,
                onTap: homeController.toggleOnline,
              ),
              const Spacer(),
              _BellButton(onTap: onNotificationsTap),
            ],
          ),
        ),

        // ─── Offline modal (Figma 2035) — floats over fully-visible map ───
        if (!state.isOnline)
          Positioned(
            left: 20,
            right: 20,
            bottom: 36,
            child: _OfflineModal(
              isLoading: state.isLoading,
              onGoOnline: homeController.goOnline,
            ),
          ),

        // ─── Incoming request bottom sheet (Figma 2034) ───────────────────
        if (state.isOnline && state.pendingRequests.isNotEmpty)
          Positioned(
            left: 0,
            right: 0,
            bottom: 0,
            child: _IncomingRequestsSheet(
              requests: state.pendingRequests,
              isLoading: state.isLoading,
              onReject: (id) => homeController.rejectBooking(id),
              onAccept: (id) => homeController.acceptBooking(id),
              onTapCard: (booking) => Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => ProviderRequestDetailScreen(
                    booking: booking,
                    homeController: homeController,
                  ),
                ),
              ),
            ),
          ),

        if (state.error != null)
          Positioned(
            top: topPadding + 64,
            left: 16,
            right: 16,
            child: _ErrorPill(message: state.error!),
          ),
      ],
    );
  }
}

// ─── Online / Offline pill (Figma 2034 top-left) ──────────────────────────────
// White pill with dark label text and a custom iOS-style toggle switch.

class _OnlineTogglePill extends StatelessWidget {
  const _OnlineTogglePill({
    required this.isOnline,
    required this.isLoading,
    required this.onTap,
  });

  final bool isOnline;
  final bool isLoading;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: isLoading ? null : onTap,
      child: Container(
        padding: const EdgeInsets.fromLTRB(14, 8, 8, 8),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(30),
          boxShadow: const [
            BoxShadow(color: Color(0x28000000), blurRadius: 10, offset: Offset(0, 3)),
          ],
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (isLoading)
              const SizedBox.square(
                dimension: 14,
                child: CircularProgressIndicator(strokeWidth: 2, color: kProviderGreen),
              )
            else
              Text(
                isOnline ? 'Go Offline' : 'Go Online',
                style: const TextStyle(
                  color: kProviderText,
                  fontWeight: FontWeight.w700,
                  fontSize: 13,
                ),
              ),
            const SizedBox(width: 8),
            _IosSwitch(isOn: isOnline),
          ],
        ),
      ),
    );
  }
}

/// Custom iOS-style toggle that animates between green (on) and grey (off).
class _IosSwitch extends StatelessWidget {
  const _IosSwitch({required this.isOn});
  final bool isOn;

  @override
  Widget build(BuildContext context) {
    return AnimatedContainer(
      duration: const Duration(milliseconds: 250),
      width: 44,
      height: 26,
      padding: const EdgeInsets.all(3),
      decoration: BoxDecoration(
        color: isOn ? kProviderGreen : const Color(0xFFD0D0D0),
        borderRadius: BorderRadius.circular(13),
      ),
      child: AnimatedAlign(
        duration: const Duration(milliseconds: 250),
        alignment: isOn ? Alignment.centerRight : Alignment.centerLeft,
        child: Container(
          width: 20,
          height: 20,
          decoration: const BoxDecoration(color: Colors.white, shape: BoxShape.circle),
        ),
      ),
    );
  }
}

// ─── Notification bell button (Figma 2034 top-right) ──────────────────────────

class _BellButton extends StatelessWidget {
  const _BellButton({this.onTap});
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        width: 42,
        height: 42,
        decoration: const BoxDecoration(
          color: Color(0xFF1A1A1A),
          shape: BoxShape.circle,
          boxShadow: [BoxShadow(color: Color(0x28000000), blurRadius: 10, offset: Offset(0, 3))],
        ),
        child: const Icon(Icons.notifications_none_rounded, color: Colors.white, size: 20),
      ),
    );
  }
}

// ─── Offline modal overlay (Figma 2035) ───────────────────────────────────────

class _OfflineModal extends StatelessWidget {
  const _OfflineModal({required this.isLoading, required this.onGoOnline});

  final bool isLoading;
  final VoidCallback onGoOnline;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.fromLTRB(24, 32, 24, 28),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(24),
        boxShadow: const [BoxShadow(color: Color(0x28000000), blurRadius: 30, offset: Offset(0, 8))],
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          // Illustration placeholder — matches 3D laptop/people art style in mockup
          Container(
            width: 130,
            height: 110,
            decoration: BoxDecoration(
              color: kProviderGreenTint,
              borderRadius: BorderRadius.circular(20),
            ),
            child: Stack(
              alignment: Alignment.center,
              children: [
                Positioned(
                  bottom: 18,
                  child: Container(
                    width: 80,
                    height: 8,
                    decoration: BoxDecoration(
                      color: kProviderGreen.withValues(alpha: 0.18),
                      borderRadius: BorderRadius.circular(4),
                    ),
                  ),
                ),
                const Icon(Icons.wifi_off_rounded, size: 54, color: kProviderGreen),
              ],
            ),
          ),
          const SizedBox(height: 20),
          const Text(
            'You are Offline!',
            style: TextStyle(
              color: kProviderText,
              fontWeight: FontWeight.w800,
              fontSize: 22,
            ),
          ),
          const SizedBox(height: 8),
          const Text(
            'Your account activity is currently set on offline, switch to online mode to start accepting trip requests.',
            textAlign: TextAlign.center,
            style: TextStyle(color: kProviderMuted, fontSize: 13, height: 1.5),
          ),
          const SizedBox(height: 24),
          SizedBox(
            width: double.infinity,
            height: 52,
            child: FilledButton(
              onPressed: isLoading ? null : onGoOnline,
              style: FilledButton.styleFrom(
                backgroundColor: kProviderGreen,
                shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
              ),
              child: isLoading
                  ? const SizedBox.square(
                      dimension: 20,
                      child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white),
                    )
                  : const Text(
                      'Go Online',
                      style: TextStyle(
                        color: Colors.white,
                        fontWeight: FontWeight.w700,
                        fontSize: 15,
                      ),
                    ),
            ),
          ),
        ],
      ),
    );
  }
}

// ─── Incoming Requests bottom sheet (Figma 2034 bottom) ───────────────────────

class _IncomingRequestsSheet extends StatelessWidget {
  const _IncomingRequestsSheet({
    required this.requests,
    required this.isLoading,
    required this.onReject,
    required this.onAccept,
    required this.onTapCard,
  });

  final List<ProviderBooking> requests;
  final bool isLoading;
  final ValueChanged<String> onReject;
  final ValueChanged<String> onAccept;
  final ValueChanged<ProviderBooking> onTapCard;

  @override
  Widget build(BuildContext context) {
    final bottomPadding = MediaQuery.of(context).padding.bottom;
    final first = requests.first;
    return Container(
      decoration: const BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
        boxShadow: [BoxShadow(color: Color(0x18000000), blurRadius: 24, offset: Offset(0, -6))],
      ),
      padding: EdgeInsets.fromLTRB(20, 20, 20, bottomPadding + 16),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'Incoming Requests...',
            style: TextStyle(
              color: kProviderText,
              fontWeight: FontWeight.w800,
              fontSize: 18,
            ),
          ),
          const Text(
            'Accept a request to start a trip.',
            style: TextStyle(color: kProviderMuted, fontSize: 13),
          ),
          const SizedBox(height: 14),
          GestureDetector(
            onTap: () => onTapCard(first),
            behavior: HitTestBehavior.opaque,
            child: ProviderRequestCard(
              booking: first,
              isLoading: isLoading,
              onReject: () => onReject(first.id),
              onAccept: () => onAccept(first.id),
            ),
          ),
          if (requests.length > 1) ...[
            const SizedBox(height: 8),
            Center(
              child: Text(
                '+${requests.length - 1} more request${requests.length > 2 ? 's' : ''}',
                style: const TextStyle(color: kProviderMuted, fontSize: 12),
              ),
            ),
          ],
        ],
      ),
    );
  }
}

// ─── Error pill ───────────────────────────────────────────────────────────────

class _ErrorPill extends StatelessWidget {
  const _ErrorPill({required this.message});
  final String message;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
      decoration: BoxDecoration(
        color: const Color(0xFFFFF0F0),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: Colors.red.shade200),
      ),
      child: Text(
        message,
        style: const TextStyle(color: Colors.red, fontSize: 13),
        textAlign: TextAlign.center,
      ),
    );
  }
}
