import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../../../shared/widgets/figma_customer_widgets.dart';
import '../../auth/data/customer_auth_api.dart';
import '../../auth/models/customer_auth_models.dart';
import '../../auth/state/customer_auth_controller.dart';
import '../../media/data/media_upload_service.dart';
import '../../support/data/support_api.dart';
import '../../wallet/data/wallet_api.dart';
import '../../wallet/ui/customer_wallet_screen.dart';
import 'customer_profile_tab.dart';

class CustomerProfileScreen extends StatefulWidget {
  const CustomerProfileScreen({
    super.key,
    required this.session,
    required this.api,
    required this.supportApi,
    required this.walletApi,
    required this.controller,
    required this.mediaUploadService,
    this.onProfileUpdated,
    this.isTabView = false,
  });

  final CustomerSession session;
  final CustomerAuthApi api;
  final SupportApi supportApi;
  final WalletApi walletApi;
  final CustomerAuthController controller;
  final MediaUploadService mediaUploadService;
  final ValueChanged<CustomerProfile>? onProfileUpdated;
  final bool isTabView;

  @override
  State<CustomerProfileScreen> createState() => _CustomerProfileScreenState();
}

class _CustomerProfileScreenState extends State<CustomerProfileScreen>
    with SingleTickerProviderStateMixin {
  late final TabController _tabController;

  CustomerProfile? _profile;
  bool _loading = true;
  ApiException? _error;

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 2, vsync: this);
    _loadProfile();
  }

  @override
  void dispose() {
    _tabController.dispose();
    super.dispose();
  }

  Future<void> _loadProfile() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final profile = await widget.api.getProfile(accessToken: widget.session.accessToken);
      setState(() {
        _profile = profile;
        _loading = false;
      });
    } on ApiException catch (e) {
      setState(() {
        _error = e;
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      body: _loading
          ? const Center(
              child: CircularProgressIndicator(color: CustomerFigmaColors.primary),
            )
          : _error != null
              ? _ErrorView(error: _error!, onRetry: _loadProfile)
              : _ProfileBody(
                  profile: _profile ?? widget.session.customer,
                  session: widget.session,
                  api: widget.api,
                  supportApi: widget.supportApi,
                  walletApi: widget.walletApi,
                  controller: widget.controller,
                  mediaUploadService: widget.mediaUploadService,
                  tabController: _tabController,
                  onProfileUpdated: (p) {
                    setState(() => _profile = p);
                    widget.onProfileUpdated?.call(p);
                  },
                ),
    );
  }
}

// ─── Profile body with Profile/Wallet tabs ────────────────────────────────────

class _ProfileBody extends StatelessWidget {
  const _ProfileBody({
    required this.profile,
    required this.session,
    required this.api,
    required this.supportApi,
    required this.walletApi,
    required this.controller,
    required this.mediaUploadService,
    required this.tabController,
    required this.onProfileUpdated,
  });

  final CustomerProfile profile;
  final CustomerSession session;
  final CustomerAuthApi api;
  final SupportApi supportApi;
  final WalletApi walletApi;
  final CustomerAuthController controller;
  final MediaUploadService mediaUploadService;
  final TabController tabController;
  final ValueChanged<CustomerProfile> onProfileUpdated;

  @override
  Widget build(BuildContext context) {
    return NestedScrollView(
      headerSliverBuilder: (context, _) => [
        SliverToBoxAdapter(
          child: Column(
            children: [
              SafeArea(
                bottom: false,
                child: Padding(
                  padding: const EdgeInsets.fromLTRB(20, 16, 20, 0),
                  child: Row(
                    children: [
                      Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: const [
                          Text(
                            'Profile',
                            style: TextStyle(
                              color: CustomerFigmaColors.text,
                              fontSize: 22,
                              fontWeight: FontWeight.w900,
                            ),
                          ),
                          Text(
                            'Manage your Personal Information and Account',
                            style: TextStyle(
                              color: CustomerFigmaColors.muted,
                              fontSize: 11,
                            ),
                          ),
                        ],
                      ),
                    ],
                  ),
                ),
              ),
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 20),
                child: TabBar(
                  controller: tabController,
                  indicatorColor: CustomerFigmaColors.primary,
                  indicatorWeight: 2.5,
                  indicatorSize: TabBarIndicatorSize.tab,
                  labelColor: CustomerFigmaColors.primary,
                  unselectedLabelColor: CustomerFigmaColors.muted,
                  labelStyle: const TextStyle(fontSize: 15, fontWeight: FontWeight.w700),
                  unselectedLabelStyle:
                      const TextStyle(fontSize: 15, fontWeight: FontWeight.w500),
                  dividerColor: CustomerFigmaColors.border,
                  tabs: const [Tab(text: 'Profile'), Tab(text: 'Wallet')],
                ),
              ),
              const SizedBox(height: 8),
            ],
          ),
        ),
      ],
      body: TabBarView(
        controller: tabController,
        children: [
          CustomerProfileTab(
            profile: profile,
            session: session,
            api: api,
            supportApi: supportApi,
            controller: controller,
            mediaUploadService: mediaUploadService,
            onProfileUpdated: onProfileUpdated,
          ),
          CustomerWalletScreen(
            session: session,
            walletApi: walletApi,
            supportApi: supportApi,
            embedded: true,
          ),
        ],
      ),
    );
  }
}

// ─── Error view ───────────────────────────────────────────────────────────────

class _ErrorView extends StatelessWidget {
  const _ErrorView({required this.error, required this.onRetry});

  final ApiException error;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline_rounded, color: Color(0xFFE53935), size: 40),
            const SizedBox(height: 16),
            Text(
              error.message,
              textAlign: TextAlign.center,
              style: const TextStyle(color: CustomerFigmaColors.text, fontSize: 14),
            ),
            const SizedBox(height: 20),
            FigmaPrimaryButton(label: 'Try again', onPressed: onRetry),
          ],
        ),
      ),
    );
  }
}
