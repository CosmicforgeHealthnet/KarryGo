import '../../../shared/widgets/async_content.dart';
import 'verification_intro_screen.dart';
import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/journey_icon.dart';
import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../disputes/state/rider_dispute_controller.dart';
import '../../vehicle/state/vehicle_controller.dart';
import '../../vehicle/ui/bike_list_screen.dart';
import '../../verification/state/verification_controller.dart';
import '../../wallet/state/wallet_controller.dart';
import '../state/provider_profile_controller.dart';
import 'profile_info_screen.dart';
import 'emergency_contact_screen.dart';
import 'guarantor_information_screen.dart';
import 'stats_settings_screen.dart';
import 'deactivate_account_screen.dart';
import 'update_profile_photo_screen.dart';

class ProfileScreen extends StatefulWidget {
  final ProviderProfileController profileController;
  final VerificationController verificationController;
  final VehicleController vehicleController;
  final RiderDisputeController? disputeController;
  final WalletController? walletController;
  final VoidCallback? onLogout;
  final VoidCallback? onAccountDeleted;

  const ProfileScreen({
    super.key,
    required this.profileController,
    required this.verificationController,
    required this.vehicleController,
    this.disputeController,
    this.walletController,
    this.onLogout,
    this.onAccountDeleted,
  });

  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _fetch();
    });
  }

  Future<void> _fetch() async {
    await Future.wait([
      widget.profileController.loadMe(),
      widget.profileController.loadStats(),
      widget.verificationController.loadVerificationStatus(),
    ]);
    if (mounted) setState(() {});
  }

  void _openProfileDetails() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) =>
            ProfileInfoScreen(profileController: widget.profileController),
      ),
    );
  }

  void _openEmergencyContact() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) =>
            EmergencyContactScreen(profileController: widget.profileController),
      ),
    );
  }

  void _openGuarantor() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => GuarantorInformationScreen(
          profileController: widget.profileController,
        ),
      ),
    );
  }

  void _openBikes() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) =>
            BikeListScreen(vehicleController: widget.vehicleController),
      ),
    );
  }

  void _openStatsSettings() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => StatsSettingsScreen(
          profileController: widget.profileController,
          verificationController: widget.verificationController,
          vehicleController: widget.vehicleController,
          walletController: widget.walletController,
          disputeController: widget.disputeController,
          onLogout: widget.onLogout,
        ),
      ),
    );
  }

  void _openAvatar() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => UpdateProfilePhotoScreen(
          profileController: widget.profileController,
        ),
      ),
    );
  }

  void _openDeactivate() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => DeactivateAccountScreen(
          profileController: widget.profileController,
          onDeleted: widget.onAccountDeleted,
        ),
      ),
    );
  }

  Future<void> _confirmLogout() async {
    if (widget.onLogout == null) return;
    final confirmed = await confirmAccountAction(
      context,
      title: 'Log out?',
      message: 'Sign in again whenever you are ready to return.',
      action: 'Log out',
      symbol: JourneySymbol.signOut,
      destructive: true,
    );
    if (confirmed && mounted) widget.onLogout?.call();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: Listenable.merge([
      widget.profileController,
      widget.verificationController,
    ]),
    builder: (context, _) {
      final profile = widget.profileController.profile;
      final name = profile?.fullName?.trim();
      final status = profile?.verificationStatus ?? 'unverified';
      final approved = status == 'approved' || status == 'verified';
      final rejected = status == 'rejected' || status == 'failed';
      final c = context.palette;
      return Scaffold(
        backgroundColor: c.background,
        body: SafeArea(
          child: RefreshIndicator(
            onRefresh: _fetch,
            child: ListView(
              padding: const EdgeInsets.only(bottom: 24),
              children: [
                DeliveryCollectionHeader(
                  title: 'Account',
                  subtitle: 'Your details. Your preferences.',
                  action: IconButton(
                    tooltip: 'Account settings',
                    onPressed: _openStatsSettings,
                    icon: const Icon(Icons.settings_outlined),
                  ),
                ),
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 16),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      if (widget.profileController.errorMessage != null)
                        Padding(
                          padding: const EdgeInsets.only(bottom: 16),
                          child: ActionNotice(
                            message: widget.profileController.errorMessage!,
                            isError: true,
                            onRetry: _fetch,
                          ),
                        ),
                      DeliveryPanel(
                        onTap: _openAvatar,
                        child: Row(
                          children: [
                            Semantics(
                              label: 'Change profile photo',
                              button: true,
                              child: SizedBox.square(
                                dimension: 68,
                                child: ClipRRect(
                                  borderRadius: BorderRadius.circular(23),
                                  child:
                                      profile?.profilePhotoUrl?.isNotEmpty ==
                                          true
                                      ? Image.network(
                                          profile!.profilePhotoUrl!,
                                          fit: BoxFit.cover,
                                          errorBuilder: (_, _, _) =>
                                              const AccountEmblem(
                                                symbol: JourneySymbol.profile,
                                                size: 68,
                                              ),
                                        )
                                      : const AccountEmblem(
                                          symbol: JourneySymbol.profile,
                                          size: 68,
                                        ),
                                ),
                              ),
                            ),
                            const SizedBox(width: 16),
                            Expanded(
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text(
                                    name == null || name.isEmpty
                                        ? 'Your rider account'
                                        : name,
                                    style: Theme.of(
                                      context,
                                    ).textTheme.titleLarge,
                                  ),
                                  const SizedBox(height: 6),
                                  Text(
                                    'Edit your photo',
                                    style: Theme.of(
                                      context,
                                    ).textTheme.bodySmall,
                                  ),
                                ],
                              ),
                            ),
                            const JourneyIcon(JourneySymbol.photo, size: 22),
                          ],
                        ),
                      ),
                      const SizedBox(height: 14),
                      DeliveryPanel(
                        padding: EdgeInsets.zero,
                        child: AccountMenuTile(
                          title: approved
                              ? 'Identity verified'
                              : rejected
                              ? 'Verification needs attention'
                              : 'Verification: ${status.replaceAll('_', ' ')}',
                          subtitle: 'View your documents and review status',
                          symbol: JourneySymbol.shield,
                          color: approved
                              ? c.success
                              : rejected
                              ? c.danger
                              : c.warning,
                          onTap: () => Navigator.of(context).push(
                            MaterialPageRoute(
                              builder: (_) => VerificationIntroScreen(
                                verificationController:
                                    widget.verificationController,
                                vehicleController: widget.vehicleController,
                              ),
                            ),
                          ),
                        ),
                      ),
                      const SizedBox(height: 24),
                      AccountSection(
                        title: 'Personal details',
                        children: [
                          AccountMenuTile(
                            title: 'Profile details',
                            subtitle: 'Name, email and location',
                            symbol: JourneySymbol.profile,
                            onTap: _openProfileDetails,
                          ),
                          AccountMenuTile(
                            title: 'Emergency contact',
                            symbol: JourneySymbol.contact,
                            onTap: _openEmergencyContact,
                          ),
                          AccountMenuTile(
                            title: 'Guarantor',
                            symbol: JourneySymbol.partner,
                            onTap: _openGuarantor,
                          ),
                        ],
                      ),
                      AccountSection(
                        title: 'Rider tools',
                        children: [
                          AccountMenuTile(
                            title: 'Vehicles',
                            subtitle: 'Manage your delivery vehicles',
                            symbol: JourneySymbol.fleet,
                            onTap: _openBikes,
                          ),
                          AccountMenuTile(
                            title: 'Settings and support',
                            subtitle: 'Documents, security and app preferences',
                            symbol: JourneySymbol.preferences,
                            onTap: _openStatsSettings,
                          ),
                        ],
                      ),
                      AccountSection(
                        title: 'Account access',
                        children: [
                          AccountMenuTile(
                            title: 'Log out',
                            symbol: JourneySymbol.signOut,
                            color: c.danger,
                            onTap: widget.onLogout == null
                                ? null
                                : _confirmLogout,
                          ),
                          AccountMenuTile(
                            title: 'Deactivate account',
                            symbol: JourneySymbol.profile,
                            color: c.danger,
                            onTap: _openDeactivate,
                          ),
                        ],
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),
        ),
      );
    },
  );
}
