import 'dart:io';

import 'package:flutter/material.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../profile/state/provider_profile_controller.dart';
import 'emergency_contact_screen.dart';
import 'guarantor_screen.dart';
import 'profile_details_screen.dart';

class SetupReviewScreen extends StatefulWidget {
  const SetupReviewScreen({
    super.key,
    required this.profileController,
    required this.profileData,
    required this.emergencyContact,
    required this.guarantorData,
    required this.operationType,
    required this.selfiePath,
    required this.onBack,
    required this.onSubmitted,
    required this.onEditPersonal,
    required this.onEditOperation,
    required this.onEditEmergency,
    required this.onEditGuarantor,
    required this.onEditPhoto,
  });

  final ProviderProfileController profileController;
  final ProfileDetailsData? profileData;
  final EmergencyContactData? emergencyContact;
  final GuarantorData? guarantorData;
  final String? operationType;
  final String? selfiePath;
  final VoidCallback onBack;
  final VoidCallback onSubmitted;
  final VoidCallback onEditPersonal;
  final VoidCallback onEditOperation;
  final VoidCallback onEditEmergency;
  final VoidCallback onEditGuarantor;
  final VoidCallback onEditPhoto;

  @override
  State<SetupReviewScreen> createState() => _SetupReviewScreenState();
}

class _SetupReviewScreenState extends State<SetupReviewScreen>
    with SingleTickerProviderStateMixin {
  bool _isLoading = false;

  /// True once we've tried to fetch/verify the profile at least once.
  bool _bootstrapDone = false;

  late final AnimationController _entrance;

  @override
  void initState() {
    super.initState();
    _entrance = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 900),
    )..forward();

    _bootstrap();
  }

  Future<void> _bootstrap() async {
    debugPrint('[REVIEW] bootstrap start | sessionProfileData=${widget.profileData != null}');

    // Always pull the freshest server state. If the user reached this screen
    // via the Setup Hub (resumed session, skipped steps), widget.profileData
    // is null and this is the only way to render real values.
    try {
      final result = await widget.profileController.loadMe();
      if (!mounted) return;
      result.when(
        success: (p) {
          debugPrint(
            '[REVIEW] loadMe OK | '
            'fullName=${p.fullName} state=${p.state} city=${p.city} '
            'email=${p.email} opType=${p.operationType} '
            'photo=${p.profilePhotoUrl} '
            'hasEC=${p.hasEmergencyContact} hasG=${p.hasGuarantor}',
          );
        },
        failure: (e) {
          debugPrint('[REVIEW] loadMe FAILED | code=${e.code} msg=${e.message}');
        },
      );
    } catch (e, st) {
      debugPrint('[REVIEW] loadMe threw: $e\n$st');
    }

    if (!mounted) return;
    setState(() => _bootstrapDone = true);

    debugPrint(
      '[REVIEW] bootstrap done | '
      'controller.fullName=${widget.profileController.profile?.fullName}',
    );
  }

  @override
  void dispose() {
    _entrance.dispose();
    super.dispose();
  }

  // ── Derived values (session first, then backend fallback) ─────────────────

  bool get _isFleet => _operationBackendValue == 'fleet';

  String get _operationLabel => _isFleet ? 'Fleet operator' : 'Own vehicle';

  String? get _operationBackendValue {
    final inMemory = widget.operationType;
    if (inMemory != null && inMemory.isNotEmpty) return inMemory;
    final live = widget.profileController.profile?.operationType;
    if (live != null && live.isNotEmpty) return live;
    return null;
  }

  String get _fullName {
    final s = widget.profileData?.fullName.trim() ?? '';
    if (s.isNotEmpty) return s;
    return widget.profileController.profile?.fullName?.trim() ?? '';
  }

  String? get _emergencyContactName {
    final s = widget.emergencyContact?.fullName.trim() ?? '';
    if (s.isNotEmpty) return s;
    final live =
        widget.profileController.emergencyContact?.fullName.trim() ?? '';
    return live.isNotEmpty ? live : null;
  }

  String? get _guarantorName {
    final s = widget.guarantorData?.fullName.trim() ?? '';
    if (s.isNotEmpty) return s;
    final live = widget.profileController.guarantor?.fullName.trim() ?? '';
    return live.isNotEmpty ? live : null;
  }

  String? get _photoPath {
    final s = widget.selfiePath;
    if (s != null && s.isNotEmpty) return s;
    return null; // backend URL is not a local file path; handled via _hasPhoto
  }

  bool get _hasPhoto {
    if ((widget.selfiePath ?? '').isNotEmpty) return true;
    final url = widget.profileController.profile?.profilePhotoUrl ?? '';
    return url.isNotEmpty;
  }

  // ── Submit ────────────────────────────────────────────────────────────────

  Future<void> _submit() async {
    if (_isLoading) return;

    final pd = widget.profileData;
    final live = widget.profileController.profile;

    final String? fullName = (pd?.fullName.trim().isNotEmpty ?? false)
        ? pd!.fullName
        : live?.fullName;
    final String? state = (pd?.state.trim().isNotEmpty ?? false)
        ? pd!.state
        : live?.state;
    final String? city = (pd?.city.trim().isNotEmpty ?? false)
        ? pd!.city
        : live?.city;
    final String? email = (pd?.email.trim().isNotEmpty ?? false)
        ? pd!.email
        : live?.email;

    if (fullName == null ||
        fullName.trim().isEmpty ||
        state == null ||
        state.trim().isEmpty ||
        city == null ||
        city.trim().isEmpty) {
      _showError(
        'Missing profile data. Please open "Basic profile" and fill it in.',
      );
      return;
    }

    setState(() => _isLoading = true);
    try {
      final result = await widget.profileController.submitOnboarding(
        fullName: fullName,
        email: email,
        state: state,
        city: city,
        operationType: _operationBackendValue ?? 'individual',
      );

      bool ok = false;
      String? err;
      result.when(
        success: (_) => ok = true,
        failure: (error) {
          if (error.code == ApiErrorCode.conflict) {
            ok = true;
          } else {
            err = error.message;
          }
        },
      );

      if (!ok) {
        if (mounted && err != null) _showError('Submit failed: $err');
        return;
      }

      widget.onSubmitted();
    } catch (e) {
      debugPrint('[REVIEW] submit failed: $e');
      if (mounted) _showError('$e'.replaceAll('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _isLoading = false);
    }
  }

  void _showError(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context)
      ..clearSnackBars()
      ..showSnackBar(
        SnackBar(
          content: Text(msg),
          backgroundColor: Colors.red.shade800,
          behavior: SnackBarBehavior.floating,
        ),
      );
  }

  void _handleBack() {
    if (_isLoading) return;
    widget.onBack();
  }

  VoidCallback? _guard(VoidCallback cb) => _isLoading ? null : cb;

  // ── Build ─────────────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final buttonGlow = c.brandTeal500.withValues(alpha: 0.20);

    // Rebuild on every controller update; also re-render once bootstrap ends.
    return AnimatedBuilder(
      animation: widget.profileController,
      builder: (context, _) {
        final String fullName = _fullName;
        final bool hasName = fullName.isNotEmpty;

        final String? ecName = _emergencyContactName;
        final String? gdName = _guarantorName;
        final bool hasPhoto = _hasPhoto;
        final String? photoPath = _photoPath;

        final List<Widget> cards = [
          _ReviewCard(
            title: 'Personal details',
            value: hasName ? fullName : 'Not provided',
            missing: !hasName,
            icon: Icons.person_rounded,
            onEdit: _guard(widget.onEditPersonal),
          ),
          _ReviewCard(
            title: 'Operation type',
            value: _operationBackendValue == null
                ? 'Not provided'
                : _operationLabel,
            missing: _operationBackendValue == null,
            icon: _isFleet
                ? Icons.local_shipping_rounded
                : Icons.directions_car_filled_rounded,
            onEdit: _guard(widget.onEditOperation),
          ),
          _ReviewCard(
            title: 'Emergency contact',
            value: ecName ?? 'Not provided',
            missing: ecName == null,
            icon: Icons.phone_in_talk_rounded,
            onEdit: _guard(widget.onEditEmergency),
          ),
          _ReviewCard(
            title: 'Guarantor details',
            value: gdName ?? 'Not provided',
            missing: gdName == null,
            icon: Icons.person_rounded,
            onEdit: _guard(widget.onEditGuarantor),
          ),
          _ReviewCard(
            title: 'Profile photo',
            value: hasPhoto ? 'Added' : 'Not added',
            missing: !hasPhoto,
            icon: Icons.photo_camera_rounded,
            photoPath: photoPath,
            onEdit: _guard(widget.onEditPhoto),
          ),
        ];

        return Scaffold(
          backgroundColor: c.background,
          body: SafeArea(
            child: Column(
              children: [
                Expanded(
                  child: !_bootstrapDone
                      ? Center(
                          child: CircularProgressIndicator(
                            color: c.brandTeal500,
                          ),
                        )
                      : SingleChildScrollView(
                          padding:
                              const EdgeInsets.symmetric(horizontal: 24),
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.stretch,
                            children: [
                              const SizedBox(height: 8),
                              _Reveal(
                                animation: _entrance,
                                begin: 0,
                                child: Row(
                                  crossAxisAlignment:
                                      CrossAxisAlignment.start,
                                  children: [
                                    GestureDetector(
                                      onTap: _handleBack,
                                      behavior: HitTestBehavior.opaque,
                                      child: Padding(
                                        padding: const EdgeInsets.symmetric(
                                          vertical: 8,
                                          horizontal: 4,
                                        ),
                                        child: Icon(
                                          Icons.arrow_back_rounded,
                                          size: 24,
                                          color: c.text,
                                        ),
                                      ),
                                    ),
                                    const SizedBox(width: 12),
                                    Expanded(
                                      child: Padding(
                                        padding:
                                            const EdgeInsets.only(top: 10),
                                        child: Column(
                                          crossAxisAlignment:
                                              CrossAxisAlignment.stretch,
                                          children: [
                                            Text(
                                              'Step 6 of 6',
                                              textAlign: TextAlign.center,
                                              style: TextStyle(
                                                fontSize: 13,
                                                fontWeight: FontWeight.w500,
                                                color: c.textMuted,
                                                height: 1.2,
                                              ),
                                            ),
                                            const SizedBox(height: 8),
                                            const _StepProgress(
                                              current: 6,
                                              total: 6,
                                            ),
                                          ],
                                        ),
                                      ),
                                    ),
                                    const SizedBox(width: 44),
                                  ],
                                ),
                              ),
                              const SizedBox(height: 28),
                              _Reveal(
                                animation: _entrance,
                                begin: 0.05,
                                child: Column(
                                  crossAxisAlignment:
                                      CrossAxisAlignment.stretch,
                                  children: [
                                    Text(
                                      'Review your information',
                                      style: TextStyle(
                                        fontSize: 24,
                                        fontWeight: FontWeight.w700,
                                        color: c.text,
                                        letterSpacing: -0.4,
                                        height: 1.2,
                                      ),
                                    ),
                                    const SizedBox(height: 10),
                                    Text(
                                      'Make sure everything is correct before you submit.',
                                      style: TextStyle(
                                        fontSize: 16,
                                        fontWeight: FontWeight.w400,
                                        color: c.textMuted,
                                        height: 1.5,
                                      ),
                                    ),
                                  ],
                                ),
                              ),
                              const SizedBox(height: 28),
                              for (int i = 0; i < cards.length; i++) ...[
                                _Reveal(
                                  animation: _entrance,
                                  begin: 0.15 + i * 0.1,
                                  child: cards[i],
                                ),
                                if (i < cards.length - 1)
                                  const SizedBox(height: 14),
                              ],
                              const SizedBox(height: 24),
                            ],
                          ),
                        ),
                ),
                Padding(
                  padding: const EdgeInsets.fromLTRB(24, 8, 24, 20),
                  child: AnimatedContainer(
                    duration: const Duration(milliseconds: 220),
                    height: 58,
                    width: double.infinity,
                    decoration: BoxDecoration(
                      borderRadius: BorderRadius.circular(12),
                      boxShadow: _isLoading
                          ? const []
                          : [
                              BoxShadow(
                                color: buttonGlow,
                                blurRadius: 20,
                                offset: const Offset(0, 8),
                              ),
                            ],
                    ),
                    child: FilledButton(
                      onPressed:
                          (_isLoading || !_bootstrapDone) ? null : _submit,
                      style: FilledButton.styleFrom(
                        backgroundColor: c.brandTeal500,
                        disabledBackgroundColor:
                            c.brandTeal500.withValues(alpha: 0.45),
                        shape: RoundedRectangleBorder(
                          borderRadius: BorderRadius.circular(12),
                        ),
                      ),
                      child: _isLoading
                          ? const SizedBox(
                              width: 22,
                              height: 22,
                              child: CircularProgressIndicator(
                                color: Colors.white,
                                strokeWidth: 2,
                              ),
                            )
                          : const Text(
                              'Submit for verification',
                              style: TextStyle(
                                fontSize: 17,
                                fontWeight: FontWeight.w700,
                                color: Colors.white,
                              ),
                            ),
                    ),
                  ),
                ),
              ],
            ),
          ),
        );
      },
    );
  }
}

// ─── Entrance reveal ────────────────────────────────────────────────────────

class _Reveal extends StatelessWidget {
  const _Reveal({
    required this.animation,
    required this.begin,
    required this.child,
  });

  final Animation<double> animation;
  final double begin;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final end = begin + 0.4 > 1 ? 1.0 : begin + 0.4;
    final interval = Interval(begin, end, curve: Curves.easeOutCubic);

    return AnimatedBuilder(
      animation: animation,
      child: child,
      builder: (context, child) {
        final t = interval.transform(animation.value);
        return Opacity(
          opacity: t,
          child: Transform.translate(
            offset: Offset(0, (1 - t) * 14),
            child: child,
          ),
        );
      },
    );
  }
}

// ─── Review row ─────────────────────────────────────────────────────────────

class _ReviewCard extends StatelessWidget {
  const _ReviewCard({
    required this.title,
    required this.value,
    required this.icon,
    required this.onEdit,
    this.photoPath,
    this.missing = false,
  });

  final String title;
  final String value;
  final IconData icon;
  final VoidCallback? onEdit;
  final String? photoPath;
  final bool missing;

  static const double _leading = 52;

  Widget _iconCircle(dynamic c) {
    return Container(
      width: _leading,
      height: _leading,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: c.navy10,
        shape: BoxShape.circle,
      ),
      child: Icon(icon, size: 26, color: c.text),
    );
  }

  Widget _buildLeading(dynamic c) {
    final path = photoPath;
    if (path == null) return _iconCircle(c);

    return SizedBox(
      width: _leading,
      height: _leading,
      child: ClipOval(
        child: Image.file(
          File(path),
          fit: BoxFit.cover,
          cacheWidth: 156,
          gaplessPlayback: true,
          errorBuilder: (_, __, ___) => _iconCircle(c),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final bool enabled = onEdit != null;

    return DecoratedBox(
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(16),
        boxShadow: [
          BoxShadow(
            color: c.brandNavy900.withValues(alpha: 0.04),
            blurRadius: 8,
            offset: const Offset(0, 2),
          ),
        ],
      ),
      child: Material(
        color: c.surface,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(16),
          side: BorderSide(color: c.border, width: 1),
        ),
        clipBehavior: Clip.antiAlias,
        child: Padding(
          padding: const EdgeInsets.fromLTRB(16, 16, 8, 16),
          child: Row(
            children: [
              _buildLeading(c),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      title,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(
                        fontSize: 14,
                        fontWeight: FontWeight.w500,
                        color: c.textMuted,
                        height: 1.3,
                      ),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      value,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(
                        fontSize: 17,
                        fontWeight:
                            missing ? FontWeight.w500 : FontWeight.w700,
                        color: missing ? c.textMuted : c.text,
                        height: 1.3,
                      ),
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 8),
              Semantics(
                button: true,
                enabled: enabled,
                label: '${missing ? 'Add' : 'Edit'} $title',
                excludeSemantics: true,
                child: InkWell(
                  onTap: onEdit,
                  borderRadius: BorderRadius.circular(10),
                  child: Padding(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 12,
                      vertical: 14,
                    ),
                    child: AnimatedOpacity(
                      opacity: enabled ? 1 : 0.5,
                      duration: const Duration(milliseconds: 150),
                      child: Text(
                        missing ? 'Add' : 'Edit',
                        style: TextStyle(
                          fontSize: 15.5,
                          fontWeight: FontWeight.w600,
                          color: c.brandTeal500,
                        ),
                      ),
                    ),
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

// ─── Step progress ──────────────────────────────────────────────────────────

class _StepProgress extends StatelessWidget {
  const _StepProgress({required this.current, required this.total});

  final int current;
  final int total;

  static const double _height = 9;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final fraction =
        total <= 0 ? 0.0 : (current / total).clamp(0.0, 1.0).toDouble();
    final previous =
        total <= 0 ? 0.0 : ((current - 1) / total).clamp(0.0, 1.0).toDouble();
    final radius = BorderRadius.circular(_height / 2);

    return SizedBox(
      height: _height,
      child: Stack(
        children: [
          Container(
            decoration: BoxDecoration(
              color: c.navy10,
              borderRadius: radius,
            ),
          ),
          TweenAnimationBuilder<double>(
            tween: Tween(begin: previous, end: fraction),
            duration: const Duration(milliseconds: 700),
            curve: Curves.easeOutCubic,
            builder: (context, value, _) {
              return FractionallySizedBox(
                alignment: Alignment.centerLeft,
                widthFactor: value,
                heightFactor: 1,
                child: Container(
                  decoration: BoxDecoration(
                    color: c.brandTeal500,
                    borderRadius: radius,
                  ),
                ),
              );
            },
          ),
        ],
      ),
    );
  }
}