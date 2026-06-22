import 'package:flutter/material.dart';

import '../../state/provider_home_controller.dart';
import '../screens/provider_request_detail_screen.dart';
import '../widgets/provider_app_colors.dart';
import '../widgets/provider_request_card.dart';

/// Requests tab — all pending (awaiting_acceptance) bookings (Figma 2049).
class ProviderRequestsScreen extends StatelessWidget {
  const ProviderRequestsScreen({
    super.key,
    required this.homeController,
    required this.state,
  });

  final ProviderHomeController homeController;
  final ProviderHomeState state;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFFF7F8F7),
      body: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // ─── Header ─────────────────────────────────────────
            const Padding(
              padding: EdgeInsets.fromLTRB(20, 16, 20, 4),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Incoming Requests',
                    style: TextStyle(
                      color: kProviderText,
                      fontWeight: FontWeight.w800,
                      fontSize: 22,
                    ),
                  ),
                  SizedBox(height: 4),
                  Text(
                    'Manage all your request in one place. Accept a request to start a trip.',
                    style: TextStyle(color: kProviderMuted, fontSize: 13, height: 1.4),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 8),
            Expanded(
              child: state.pendingRequests.isEmpty
                  ? _EmptyState(isOnline: state.isOnline)
                  : ListView.builder(
                      padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                      itemCount: state.pendingRequests.length,
                      itemBuilder: (context, i) {
                        final booking = state.pendingRequests[i];
                        return Padding(
                          padding: const EdgeInsets.only(bottom: 16),
                          child: GestureDetector(
                            onTap: () => Navigator.of(context).push(
                              MaterialPageRoute(
                                builder: (_) => ProviderRequestDetailScreen(
                                  booking: booking,
                                  homeController: homeController,
                                ),
                              ),
                            ),
                            child: ProviderRequestCard(
                              booking: booking,
                              isLoading: state.isLoading,
                              onReject: () => homeController.rejectBooking(booking.id),
                              onAccept: () => homeController.acceptBooking(booking.id),
                            ),
                          ),
                        );
                      },
                    ),
            ),
          ],
        ),
      ),
    );
  }
}

class _EmptyState extends StatelessWidget {
  const _EmptyState({required this.isOnline});
  final bool isOnline;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(40),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              isOnline ? Icons.inbox_rounded : Icons.wifi_off_rounded,
              color: kProviderMuted,
              size: 56,
            ),
            const SizedBox(height: 16),
            const Text(
              'No pending requests',
              style: TextStyle(color: kProviderText, fontWeight: FontWeight.w700, fontSize: 16),
            ),
            const SizedBox(height: 8),
            Text(
              isOnline
                  ? 'Waiting for customers to book a truck...'
                  : 'Go online on the Home tab to receive requests.',
              style: const TextStyle(color: kProviderMuted, fontSize: 13),
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    );
  }
}
