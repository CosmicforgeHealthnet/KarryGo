import 'package:flutter/material.dart';

import '../../../../core/widgets/karryam_ui.dart';
import '../../state/provider_home_controller.dart';
import '../screens/provider_request_detail_screen.dart';
import '../widgets/provider_request_card.dart';

/// Requests tab — every booking waiting for this driver's answer.
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
    final requests = state.pendingRequests;
    return Scaffold(
      body: SafeArea(
        bottom: false,
        child: CustomScrollView(
          slivers: [
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(20, 24, 20, 8),
              sliver: SliverToBoxAdapter(
                child: KHeading(
                  eyebrow: 'Find your next move',
                  title: 'Available requests',
                  subtitle: !state.isOnline
                      ? "You're offline. Go online on Home to receive loads."
                      : requests.isEmpty
                          ? 'Loads waiting for your answer will appear here.'
                          : '${requests.length} ${requests.length == 1 ? 'load is' : 'loads are'} waiting for your answer. Open one to see the full details.',
                ),
              ),
            ),
            if (state.error != null)
              SliverPadding(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 0),
                sliver: SliverToBoxAdapter(child: KNotice(state.error!, tone: KTone.danger)),
              ),
            if (requests.isEmpty)
              SliverFillRemaining(
                hasScrollBody: false,
                child: Center(
                  child: KEmptyState(
                    icon: state.isOnline ? Icons.inbox_rounded : Icons.wifi_off_rounded,
                    title: state.isOnline ? 'No requests right now' : "You're offline",
                    message: state.isOnline
                        ? "We'll show new loads here the moment a customer books a truck near you."
                        : 'Switch to online on the Home tab to start receiving haulage requests.',
                    actionLabel: state.isOnline ? null : 'Go online',
                    onAction: state.isOnline ? null : homeController.goOnline,
                  ),
                ),
              )
            else
              SliverPadding(
                padding: const EdgeInsets.fromLTRB(20, 16, 20, 24),
                sliver: SliverList.separated(
                  itemCount: requests.length,
                  separatorBuilder: (_, _) => const SizedBox(height: 14),
                  itemBuilder: (context, i) {
                    final booking = requests[i];
                    return ProviderRequestCard(
                      booking: booking,
                      isLoading: state.isLoading,
                      onOpen: () => Navigator.of(context).push(
                        MaterialPageRoute(
                          builder: (_) => ProviderRequestDetailScreen(
                            booking: booking,
                            homeController: homeController,
                          ),
                        ),
                      ),
                      onReject: () => homeController.rejectBooking(booking.id),
                      onAccept: () => homeController.acceptBooking(booking.id),
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
