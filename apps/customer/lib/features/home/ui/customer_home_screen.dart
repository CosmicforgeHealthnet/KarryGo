import 'package:flutter/material.dart';

import '../../../app/app_routes.dart';
import '../../../shared/widgets/figma_customer_widgets.dart';
import '../../auth/data/customer_auth_api.dart';
import '../../auth/state/customer_auth_controller.dart';
import '../../hauling/data/places_api.dart';
import '../../hauling/state/hauling_booking_controller.dart';
import '../../media/data/media_upload_service.dart';
import '../../notifications/state/notification_controller.dart';
import '../../profile/ui/customer_profile_screen.dart';
import '../../support/data/support_api.dart';
import '../../wallet/data/wallet_api.dart';
import 'tabs/customer_home_tab.dart';
import 'tabs/customer_notifications_tab.dart';
import 'tabs/customer_trips_tab.dart';
import 'widgets/customer_home_bottom_nav.dart';

class CustomerHomeScreen extends StatefulWidget {
  const CustomerHomeScreen({
    super.key,
    required this.controller,
    required this.state,
    required this.authApi,
    required this.supportApi,
    required this.walletApi,
    required this.mediaUploadService,
    required this.haulingController,
    required this.placesApi,
    required this.notificationController,
  });

  final CustomerAuthController controller;
  final CustomerAuthState state;
  final CustomerAuthApi authApi;
  final SupportApi supportApi;
  final WalletApi walletApi;
  final MediaUploadService mediaUploadService;
  final HaulingBookingController haulingController;
  final PlacesApi placesApi;
  final NotificationController notificationController;

  @override
  State<CustomerHomeScreen> createState() => _CustomerHomeScreenState();
}

class _CustomerHomeScreenState extends State<CustomerHomeScreen> {
  int _selectedIndex = 0;

  @override
  Widget build(BuildContext context) {
    final session = widget.state.session;

    return Scaffold(
      key: const ValueKey(CustomerAppRoutes.home),
      backgroundColor: CustomerFigmaColors.surface,
      body: IndexedStack(
        index: _selectedIndex,
        children: [
          CustomerHomeTab(
            state: widget.state,
            controller: widget.controller,
            haulingController: widget.haulingController,
            placesApi: widget.placesApi,
          ),
          const CustomerTripsTab(),
          CustomerNotificationsTab(controller: widget.notificationController),
          session == null
              ? const _ProfilePlaceholder()
              : CustomerProfileScreen(
                  session: session,
                  api: widget.authApi,
                  supportApi: widget.supportApi,
                  walletApi: widget.walletApi,
                  controller: widget.controller,
                  mediaUploadService: widget.mediaUploadService,
                  isTabView: true,
                ),
        ],
      ),
      bottomNavigationBar: CustomerHomeBottomNav(
        selectedIndex: _selectedIndex,
        onTap: (i) => setState(() => _selectedIndex = i),
      ),
    );
  }
}

class _ProfilePlaceholder extends StatelessWidget {
  const _ProfilePlaceholder();

  @override
  Widget build(BuildContext context) {
    return const Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      body: Center(
        child: CircularProgressIndicator(color: CustomerFigmaColors.primary),
      ),
    );
  }
}
