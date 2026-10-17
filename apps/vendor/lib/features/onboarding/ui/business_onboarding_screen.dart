import 'branch_location_picker.dart';
import 'package:latlong2/latlong.dart';
import 'dart:async';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/widgets/vendor_widgets.dart';
import '../data/document_upload_client.dart';
import 'branch_setup_screen.dart';
import 'setup_design.dart';
import '../../../core/widgets/nigerian_phone_prefix.dart';
import '../state/setup_progress.dart';
import '../state/business_onboarding_controller.dart';
import '../../orders/ui/order_feedback_dialog.dart';

String _label(Object? value) =>
    value
        ?.toString()
        .toLowerCase()
        .split('_')
        .map((v) => v.isEmpty ? v : '${v[0].toUpperCase()}${v.substring(1)}')
        .join(' ') ??
    '';

Future<void> _showSetupFeedback(
  BuildContext context,
  String title,
  String message,
) async {
  await showModalBottomSheet<void>(
    context: context,
    isScrollControlled: true,
    showDragHandle: true,
    builder: (context) => SafeArea(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(28, 12, 28, 28),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TweenAnimationBuilder<double>(
              tween: Tween(begin: .65, end: 1),
              duration: const Duration(milliseconds: 400),
              curve: Curves.easeOutBack,
              builder: (context, value, child) =>
                  Transform.scale(scale: value, child: child),
              child: Container(
                width: 76,
                height: 76,
                decoration: BoxDecoration(
                  color: KarryamColors.teal.withValues(alpha: .12),
                  shape: BoxShape.circle,
                ),
                child: Icon(
                  Icons.task_alt_rounded,
                  size: 40,
                  color: KarryamColors.tealInk.forInk(context),
                ),
              ),
            ),
            const SizedBox(height: 20),
            Text(
              title,
              textAlign: TextAlign.center,
              style: const TextStyle(fontSize: 25, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 10),
            Text(
              message,
              textAlign: TextAlign.center,
              style: TextStyle(
                color: KarryamColors.gray.forInk(context),
                height: 1.5,
              ),
            ),
            const SizedBox(height: 24),
            PrimaryAction(
              label: 'Continue',
              onPressed: () => Navigator.pop(context),
            ),
          ],
        ),
      ),
    ),
  );
}

Future<void> openBusinessBankDetails(
  BuildContext context,
  VendorRuntime runtime,
) async {
  final controller = BusinessOnboardingController(runtime.backend);
  try {
    final loaded = await controller.loadBankProfile();
    if (!context.mounted) return;
    if (!loaded) {
      await showOrderFeedback(
        context,
        title: 'Could not load bank details',
        message: controller.error ?? 'Try opening bank details again.',
        success: false,
      );
      return;
    }
    await Navigator.of(context).push(
      setupRoute<void>(
        _SetupForm(
          controller: controller,
          runtime: runtime,
          kind: 'bank',
          onSettings: () {},
        ),
      ),
    );
  } finally {
    controller.dispose();
  }
}

class BusinessOnboardingScreen extends StatefulWidget {
  const BusinessOnboardingScreen({
    super.key,
    required this.runtime,
    required this.onSettings,
    this.onActivated,
    this.onSubmitted,
  });
  final VendorRuntime runtime;
  final VoidCallback onSettings;
  final VoidCallback? onActivated;
  final VoidCallback? onSubmitted;
  @override
  State<BusinessOnboardingScreen> createState() =>
      _BusinessOnboardingScreenState();
}

class _BusinessOnboardingScreenState extends State<BusinessOnboardingScreen> {
  late final controller = BusinessOnboardingController(widget.runtime.backend);
  @override
  void initState() {
    super.initState();
    unawaited(_initialize());
  }

  Future<void> _initialize() async {
    await controller.load();
    if (mounted && controller.needsBusiness) await _edit('business');
  }

  @override
  void dispose() {
    controller.dispose();
    super.dispose();
  }

  Future<void> _edit(String kind, {JsonObject? record}) async {
    await Navigator.of(context).push(
      setupRoute<void>(
        _SetupForm(
          controller: controller,
          runtime: widget.runtime,
          kind: kind,
          record: record,
          journey: true,
          onSettings: widget.onSettings,
        ),
      ),
    );
    if (mounted) await controller.load();
    if (mounted && controller.submitted && !controller.blocked) {
      widget.onSubmitted?.call();
    }
  }

  Future<void> _submit(bool activation) async {
    if (!canFinishSetup(controller)) return;
    final success = await controller.save(
      'POST',
      '/api/v1/vendor/onboarding/${activation ? 'request-activation' : 'submit'}',
      {},
    );
    if (success) {
      await controller.load();
      if (!mounted) return;
      await _showSetupFeedback(
        context,
        controller.activated
            ? 'Your business is ready'
            : activation
            ? 'Activation requested'
            : 'Business submitted',
        controller.activated
            ? 'Your setup is complete. Let?s get started.'
            : activation
            ? 'Your activation request has been received. Return here to track its status.'
            : 'Your business has been submitted for review. Your documents do not need to be submitted again. Return here to track each check.',
      );
      if (!mounted) return;
      if (controller.activated) {
        widget.onActivated?.call();
      } else if (controller.submitted && !controller.blocked) {
        widget.onSubmitted?.call();
      }
    }
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: controller,
    builder: (context, _) => VendorScreenFrame(
      modeLabel: widget.runtime.mode.name,
      onSettings: widget.onSettings,
      leading: Text(
        'YOUR BUSINESS',
        style: TextStyle(
          fontSize: 11,
          fontWeight: FontWeight.w700,
          letterSpacing: 2,
          color: KarryamColors.navy.forInk(context),
        ),
      ),
      body: RefreshIndicator(
        onRefresh: controller.load,
        color: KarryamColors.tealInk.forInk(context),
        child: ListView(
          padding: const EdgeInsets.fromLTRB(24, 20, 24, 24),
          children: [
            if (!controller.loaded) ...[
              const Text(
                "Let us get your business ready.",
                style: TextStyle(
                  fontSize: 32,
                  fontWeight: FontWeight.w700,
                  height: 1.12,
                ),
              ),
              const SizedBox(height: 28),
              if (controller.busy)
                Center(
                  child: CircularProgressIndicator(
                    color: KarryamColors.tealInk.forInk(context),
                  ),
                ),
              if (controller.error != null) ...[
                InlineNotice(message: controller.error!, error: true),
                const SizedBox(height: 16),
                PrimaryAction(label: 'Try again', onPressed: controller.load),
              ],
            ] else if (controller.needsBusiness) ...[
              const SetupChapterHeader(kind: 'business'),
              const SizedBox(height: 24),
              PrimaryAction(
                label: 'Set up your business',
                onPressed: controller.busy ? null : () => _edit('business'),
              ),
            ] else ...[
              Text(
                controller.activated
                    ? "You are ready to move."
                    : controller.blocked
                    ? 'Your business needs attention.'
                    : controller.submitted
                    ? 'Your business is under review.'
                    : 'Your next chapter starts here.',
                style: const TextStyle(
                  fontSize: 38,
                  letterSpacing: -1.8,
                  height: 1.08,
                  fontWeight: FontWeight.w700,
                ),
              ),
              const SizedBox(height: 12),
              Text(
                controller.business['trading_name']?.toString().isNotEmpty ==
                        true
                    ? controller.business['trading_name'].toString()
                    : controller.business['legal_name'].toString(),
                style: TextStyle(
                  color: KarryamColors.gray.forInk(context),
                  fontSize: 15,
                ),
              ),
              const SizedBox(height: 28),
              _ProgressCard(controller: controller),
              const SizedBox(height: 24),
              if (controller.error != null) ...[
                InlineNotice(message: controller.error!, error: true),
                const SizedBox(height: 16),
              ],
              if (controller.activated)
                const InlineNotice(
                  message: 'Your business is activated. Your setup is saved.',
                ),
              if (controller.blocked)
                const InlineNotice(
                  message:
                      'This business cannot be activated in its current status. Contact your business administrator.',
                  error: true,
                ),
              const SizedBox(height: 12),
              const SetupSectionLabel(
                label: 'MAKE IT YOURS',
                caption: 'Your business, brought together.',
              ),
              _step(
                'Business details',
                'Legal name, contact and business type',
                Icons.storefront_outlined,
                () => _edit('business'),
              ),
              _step(
                'Authorized representative',
                controller.representative.isEmpty
                    ? 'Introduce the person representing your business'
                    : '${controller.representative['first_name']} ${controller.representative['last_name']}',
                Icons.person_outline_rounded,
                () => _edit('representative'),
              ),
              _step(
                'Business types',
                '${controller.capabilities.length} business types added',
                Icons.category_outlined,
                () => _edit('capabilities'),
              ),
              _step(
                'Brands & branches',
                '${controller.branches.length} branches / ${controller.brands.length} brands',
                Icons.location_on_outlined,
                () => _edit('locations'),
              ),
              _step(
                'Documents & verification',
                '${controller.documents.length} documents / track every review',
                Icons.verified_user_outlined,
                () => _edit('verification'),
              ),
              const SizedBox(height: 24),
              if (controller.submitted &&
                  !controller.blocked &&
                  widget.onSubmitted != null) ...[
                PrimaryAction(
                  label: 'Open dashboard',
                  onPressed: controller.busy ? null : widget.onSubmitted,
                ),
                const SizedBox(height: 16),
              ],
              if (!controller.blocked && !controller.activated) ...[
                PrimaryAction(
                  label: controller.readiness['ready'] == true
                      ? 'Request activation'
                      : controller.submitted
                      ? 'Business under review'
                      : 'Submit business for review',
                  busy: controller.busy,
                  onPressed: !canFinishSetup(controller)
                      ? null
                      : () => _submit(controller.readiness['ready'] == true),
                ),
                const SizedBox(height: 10),
                Text(
                  controller.submitted
                      ? 'Your submission is received. Track your checks above. Activation becomes available when the required checks are approved.'
                      : 'Complete each step and submit the required documents before continuing. Activation becomes available when your checks are approved.',
                  textAlign: TextAlign.center,
                  style: TextStyle(
                    fontSize: 12,
                    color: KarryamColors.gray.forInk(context),
                    height: 1.5,
                  ),
                ),
              ],
            ],
            const SizedBox(height: 18),
            TextButton(
              onPressed: controller.busy ? null : widget.runtime.signOut,
              child: const Text('Sign out'),
            ),
          ],
        ),
      ),
    ),
  );
  Widget _step(
    String title,
    String subtitle,
    IconData icon,
    VoidCallback action,
  ) => SetupJourneyRow(
    title: title,
    subtitle: subtitle,
    icon: icon,
    progress: stepProgress(controller, switch (title) {
      'Business details' => 'business',
      'Authorized representative' => 'representative',
      'Business types' => 'capabilities',
      'Brands & branches' => 'locations',
      'Bank details' => 'bank',
      _ => 'verification',
    }),
    onTap: controller.busy || controller.blocked ? null : action,
  );
}

class _ProgressCard extends StatelessWidget {
  const _ProgressCard({required this.controller});
  final BusinessOnboardingController controller;
  @override
  Widget build(BuildContext context) => SetupPassport(
    progress: ((controller.readiness['progress_percent'] as num?) ?? 0)
        .clamp(0, 100)
        .toDouble(),
    // The server says NOT_STARTED until the business is submitted, even when
    // most steps are done; show what the merchant actually sees.
    status:
        controller.overview['status'] == 'NOT_STARTED' &&
            ((controller.readiness['progress_percent'] as num?) ?? 0) > 0
        ? 'In progress'
        : _label(controller.overview['status']),
    caption: controller.activated
        ? 'Everything is in place.'
        : controller.readiness['ready'] == true
        ? 'Your checks are complete. Request activation when you are ready.'
        : 'A little groundwork. A whole new beginning.',
  );
}

class _SetupForm extends StatefulWidget {
  const _SetupForm({
    required this.controller,
    required this.runtime,
    required this.kind,
    required this.onSettings,
    this.record,
    this.journey = false,
  });
  final BusinessOnboardingController controller;
  final VendorRuntime runtime;
  final String kind;
  final JsonObject? record;
  final bool journey;
  final VoidCallback onSettings;
  @override
  State<_SetupForm> createState() => _SetupFormState();
}

class _SetupFormState extends State<_SetupForm> {
  final _form = GlobalKey<FormState>();
  final Map<String, TextEditingController> _fields = {};
  final Map<String, String> _fieldErrors = {};
  late final JsonObject _record = Map.of(
    widget.record ??
        switch (widget.kind) {
          'business' => c.business,
          'representative' => c.representative,
          'bank' => c.bank,
          _ => <String, dynamic>{},
        },
  );
  String _capacity = 'OWNER',
      _capability = '',
      _dimension = 'BUSINESS',
      _subject = '',
      _brand = '';
  bool _authorized = false, _working = false;
  String? _activeAction;
  // Which action produced [_error], kept after the action finishes.
  String? _errorAction;
  String? _error, _challenge, _developmentCode;
  DateTime? _challengeExpires;
  String? _pendingMedia;
  final Set<String> _selectedDocs = {};
  Uint8List? _fileBytes;
  String? _fileName, _mime;
  BusinessOnboardingController get c => widget.controller;
  bool get _mock => widget.runtime.mode == VendorDataMode.mock;
  @override
  void initState() {
    super.initState();
    _capacity = _record['capacity'] as String? ?? 'OWNER';
    _authorized = _record['authorized'] == true || (_mock && _record.isEmpty);
    if (_mock && widget.kind == 'capabilities') {
      final available = c.catalogue.where(
        (item) => !c.capabilities.any(
          (cap) => cap['capability_type'] == item['type'],
        ),
      );
      if (available.isNotEmpty) {
        _capability = available.first['type'].toString();
      }
    }
    _brand = _record['brand_id'] as String? ?? '';
    if (_mock && widget.kind == 'verification') unawaited(_demoDocument());
    if (widget.kind == 'verification' && _record.isEmpty) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!mounted) return;
        for (final r in c.requirements) {
          if (r['satisfied'] != true &&
              !reviewPending(r['status']) &&
              r['dimension'] != null &&
              r['dimension'] != '' &&
              _requirementSubject(r) != null) {
            _chooseRequirement(r);
            break;
          }
        }
      });
    }

    if (widget.kind == 'verification' && _record['dimension'] != null) {
      _dimension = _record['dimension'];
      _subject = _record['representative_id'] != null
          ? 'REPRESENTATIVE:${_record['representative_id']}'
          : _record['branch_id'] != null
          ? 'BRANCH:${_record['branch_id']}'
          : _record['brand_id'] != null
          ? 'BRAND:${_record['brand_id']}'
          : 'BUSINESS:${c.businessId}';
    }
  }

  @override
  void dispose() {
    for (final field in _fields.values) {
      field.clear();
      field.dispose();
    }
    _fileBytes = null;
    super.dispose();
  }

  String _initialValue(String key, Object? initial) {
    final value = _rawInitialValue(key, initial);
    return isPhoneFieldKey(key) ? nigerianPhoneLocal(value) : value;
  }

  String _rawInitialValue(String key, Object? initial) {
    final value = (initial ?? _record[key])?.toString() ?? '';
    // Karryam operates in Nigeria only, so these are fixed, never typed.
    if (value.isEmpty) {
      if (key == 'currency') return 'NGN';
      if (key == 'timezone') return 'Africa/Lagos';
      if (key == 'country_code') return 'NG';
    }
    if (!_mock || value.isNotEmpty) return value;
    return switch (key) {
      'legal_name' => 'Karryam Demo Business',
      'trading_name' => 'Karryam Kitchen',
      'support_email' || 'email' => 'owner@example.com',
      'support_phone' || 'phone' => '+2348012345678',
      'currency' => 'NGN',
      'timezone' => 'Africa/Lagos',
      'first_name' => 'Ada',
      'last_name' => 'Okafor',
      'ownership_percent' => '100',
      'bank_code' => '044',
      'account_number' => '0123456789',
      'security_code' => '123456',
      'display_name' =>
        widget.kind == 'branch'
            ? 'Lagos branch ${c.branches.length + 1}'
            : 'Karryam Kitchen ${c.brands.length + 1}',
      'description' => 'Fresh meals and everyday essentials.',
      'line1' => '10 Example Road',
      'line2' => 'Ground floor',
      'city' || 'region' => 'Lagos',
      'postal_code' => '100001',
      'country_code' => 'NG',
      'latitude' => '6.5244',
      'longitude' => '3.3792',
      'document_type' => 'OTHER_SUPPORTING_DOCUMENT',
      _ => '',
    };
  }

  TextEditingController _field(String key, {Object? initial}) =>
      _fields.putIfAbsent(
        key,
        () => TextEditingController(text: _initialValue(key, initial)),
      );

  Future<void> _demoDocument() async {
    final asset = await rootBundle.load('assets/onboarding/demo-document.pdf');
    if (!mounted) return;
    c.beginNewUpload();
    final type = _field('document_type');
    if (type.text.trim().isEmpty) type.text = 'OTHER_SUPPORTING_DOCUMENT';
    setState(() {
      _fileBytes = asset.buffer.asUint8List(
        asset.offsetInBytes,
        asset.lengthInBytes,
      );
      _fileName = 'karryam-demo-document.pdf';
      _mime = 'application/pdf';
      _pendingMedia = null;
    });
  }

  Widget _input(
    String key,
    String label, {
    Object? initial,
    bool required = true,
    bool number = false,
    int? min,
    int? max,
    bool email = false,
    bool secret = false,
  }) => Padding(
    padding: const EdgeInsets.only(bottom: 18),
    child: TextFormField(
      controller: _field(key, initial: initial),
      enabled: !_working,
      keyboardType: isPhoneFieldKey(key)
          ? TextInputType.phone
          : number
          ? const TextInputType.numberWithOptions(decimal: true, signed: true)
          : email
          ? TextInputType.emailAddress
          : TextInputType.text,
      autocorrect: !secret && !email,
      enableSuggestions: !secret && !email,
      obscureText: secret,
      decoration: InputDecoration(
        labelText: label,
        errorText: _fieldErrors[key],
        prefixIcon: isPhoneFieldKey(key) ? nigerianPhonePrefix : null,
        hintText: isPhoneFieldKey(key) ? nigerianPhoneHint : null,
      ),
      onChanged: (_) {
        if (_fieldErrors.containsKey(key)) {
          setState(() => _fieldErrors.remove(key));
        }
      },
      validator: (v) {
        final text = v?.trim() ?? '';
        if (isPhoneFieldKey(key)) {
          return validateNigerianPhone(text, required: required);
        }
        if (required && text.isEmpty) return 'Enter $label';
        if (text.isEmpty) return null;
        if (min != null && text.length < min) {
          return 'Use at least $min characters';
        }
        if (max != null && text.length > max) {
          return 'Use at most $max characters';
        }
        if (number && num.tryParse(text) == null) return 'Enter a valid number';
        if (email && !RegExp(r'^[^\s@]+@[^\s@]+\.[^\s@]+$').hasMatch(text)) {
          return 'Enter a valid email';
        }
        return null;
      },
    ),
  );
  static const _documentTypes = <String, Map<String, String>>{
    'IDENTITY': {
      'NIN_SLIP': 'NIN slip or card',
      'INTERNATIONAL_PASSPORT': 'International passport',
      'DRIVERS_LICENCE': "Driver's licence",
      'VOTERS_CARD': "Voter's card",
    },
    'BUSINESS': {
      'CAC_CERTIFICATE': 'CAC certificate',
      'CAC_STATUS_REPORT': 'CAC status report',
      'BUSINESS_NAME_CERTIFICATE': 'Business name certificate',
      'TIN_CERTIFICATE': 'Tax (TIN) certificate',
    },
    'LOCATION': {
      'UTILITY_BILL': 'Utility bill (electricity, water)',
      'TENANCY_AGREEMENT': 'Tenancy or lease agreement',
      'SHOP_PHOTO': 'Photo of the shop front',
      'BANK_STATEMENT': 'Bank statement showing the address',
    },
    'BRAND': {
      'TRADEMARK_CERTIFICATE': 'Trademark certificate',
      'BRAND_AUTHORIZATION': 'Brand authorization letter',
    },
    'FOOD_KITCHEN': {
      'NAFDAC_CERTIFICATE': 'NAFDAC certificate',
      'FOOD_HANDLER_CERTIFICATE': "Food handler's certificate",
      'HEALTH_PERMIT': 'Health or sanitation permit',
      'KITCHEN_PHOTO': 'Photo of the kitchen',
    },
    'BANK': {
      'BANK_STATEMENT': 'Bank statement',
      'BANK_REFERENCE_LETTER': 'Bank reference letter',
    },
  };

  /// Document type picker for the current check, backed by the same
  /// 'document_type' field the upload submits.
  Widget _documentTypeField() {
    final field = _field('document_type');
    final options = {
      ...?_documentTypes[_dimension.toString()],
      'OTHER_SUPPORTING_DOCUMENT': 'Other supporting document',
    };
    final current = field.text.trim();
    return Padding(
      padding: const EdgeInsets.only(bottom: 18),
      child: DropdownButtonFormField<String>(
        key: ValueKey('document-type-$_dimension-$current'),
        isExpanded: true,
        initialValue: options.containsKey(current) ? current : null,
        decoration: InputDecoration(
          labelText: 'Document type',
          errorText: _fieldErrors['document_type'],
        ),
        items: [
          for (final e in options.entries)
            DropdownMenuItem(value: e.key, child: Text(e.value)),
        ],
        validator: (v) => v == null ? 'Choose the type of document' : null,
        onChanged: _working
            ? null
            : (v) => setState(() {
                field.text = v ?? '';
                _fieldErrors.remove('document_type');
              }),
      ),
    );
  }

  static const _nigerianStates = [
    'Abia',
    'Adamawa',
    'Akwa Ibom',
    'Anambra',
    'Bauchi',
    'Bayelsa',
    'Benue',
    'Borno',
    'Cross River',
    'Delta',
    'Ebonyi',
    'Edo',
    'Ekiti',
    'Enugu',
    'Federal Capital Territory',
    'Gombe',
    'Imo',
    'Jigawa',
    'Kaduna',
    'Kano',
    'Katsina',
    'Kebbi',
    'Kogi',
    'Kwara',
    'Lagos',
    'Nasarawa',
    'Niger',
    'Ogun',
    'Ondo',
    'Osun',
    'Oyo',
    'Plateau',
    'Rivers',
    'Sokoto',
    'Taraba',
    'Yobe',
    'Zamfara',
  ];

  /// State picker backed by the same 'region' field the form submits.
  Widget _stateField(Object? initial) {
    final field = _field('region', initial: initial);
    final current = field.text.trim();
    final match = _nigerianStates
        .where((s) => s.toLowerCase() == current.toLowerCase())
        .firstOrNull;
    return Padding(
      padding: const EdgeInsets.only(bottom: 18),
      child: DropdownButtonFormField<String>(
        isExpanded: true,
        initialValue: match,
        decoration: InputDecoration(
          labelText: 'State',
          errorText: _fieldErrors['region'],
        ),
        items: [
          for (final state in _nigerianStates)
            DropdownMenuItem(value: state, child: Text(state)),
        ],
        validator: (v) => v == null ? 'Choose your state' : null,
        onChanged: _working
            ? null
            : (v) => setState(() {
                field.text = v ?? '';
                _fieldErrors.remove('region');
              }),
      ),
    );
  }

  String _text(String key) {
    final value = _fields[key]?.text.trim() ?? '';
    return isPhoneFieldKey(key) ? nigerianPhoneForSubmit(value) : value;
  }

  Future<void> _task(
    Future<void> Function() action, [
    String activity = 'save',
  ]) async {
    if (_working) return;
    setState(() {
      _working = true;
      _activeAction = activity;
      _error = null;
      _errorAction = null;
      _fieldErrors.clear();
    });
    c.error = null;
    try {
      await action();
    } catch (e) {
      if (mounted) {
        setState(() {
          _errorAction = activity;
          if (e is ApiException) {
            for (final field in e.fields) {
              final key = field.field.startsWith('address.')
                  ? field.field.substring('address.'.length)
                  : field.field;
              _fieldErrors[key] = field.message;
            }
            _error = e.fields.isEmpty
                ? e.message
                : e.fields.map((field) => field.message).toSet().join('\n');
          } else {
            _error = e is FormatException
                ? e.message
                : c.error ??
                      'This step could not be completed. Please try again.';
          }
        });
      }
    } finally {
      if (mounted) {
        setState(() {
          _working = false;
          _activeAction = null;
        });
      }
    }
  }

  Future<void> _advance() async {
    await c.load();
    if (!mounted) return;
    if (c.error != null) throw FormatException(c.error!);
    const steps = [
      'business',
      'representative',
      'capabilities',
      'locations',
      'verification',
    ];
    final index = steps.indexOf(widget.kind);
    if (index < 0 || index == steps.length - 1) {
      Navigator.pop(context);
      return;
    }
    await Navigator.of(context).pushReplacement(
      setupRoute<void>(
        _SetupForm(
          controller: c,
          runtime: widget.runtime,
          kind: steps[index + 1],
          journey: true,
          onSettings: widget.onSettings,
        ),
      ),
    );
  }

  Future<void> _saved(
    String method,
    String path,
    JsonObject body, {
    Map<String, String> params = const {},
  }) async {
    final success = await c.save(method, path, body, params: params);
    if (!success) {
      if (c.saveError != null) throw c.saveError!;
      throw const FormatException(
        'Saving failed. Refresh if this record has changed.',
      );
    }
    if (!mounted) return;
    if (widget.journey) {
      await _advance();
    } else {
      Navigator.pop(context);
    }
  }

  Future<void> _save() => _task(() async {
    if (!_form.currentState!.validate()) return;
    final params = {'businessId': c.businessId};
    final version = (_record['version'] as num?)?.toInt() ?? 0;
    switch (widget.kind) {
      case 'business':
        final creating = c.needsBusiness;
        await _saved(
          creating ? 'POST' : 'PATCH',
          creating
              ? '/api/v1/vendor/businesses'
              : '/api/v1/vendor/businesses/{businessId}',
          {
            for (final key in [
              'legal_name',
              'trading_name',
              'support_email',
              'support_phone',
              'currency',
              'timezone',
            ])
              key: _text(key),
            if (!creating) 'expected_version': version,
          },
          params: creating ? const {} : params,
        );
      case 'representative':
        final replacing = c.verifications.any(
          (v) => v['dimension'] == 'IDENTITY' && v['status'] == 'VERIFIED',
        );
        final grant = replacing
            ? await _securityGrant('onboarding.representative_replace')
            : null;
        if (replacing && grant == null) return;
        final ownership = _text('ownership_percent');
        if (ownership.isNotEmpty &&
            (num.parse(ownership) < 0 || num.parse(ownership) > 100)) {
          throw const FormatException('Ownership must be between 0 and 100.');
        }
        await _saved('PUT', '/api/v1/vendor/onboarding/representative', {
          'capacity': _capacity,
          'first_name': _text('first_name'),
          'last_name': _text('last_name'),
          'email': _text('email'),
          'phone': _text('phone'),
          'authorized': _authorized,
          if (ownership.isNotEmpty) 'ownership_percent': num.parse(ownership),
          'expected_version': version,
          'step_up_grant_id': ?grant,
        });
      case 'bank':
        final grant = await _securityGrant('finance.bank_change');
        if (grant == null) return;
        await _saved('PUT', '/api/v1/vendor/onboarding/bank-profile', {
          'bank_code': _text('bank_code'),
          'account_number': _text('account_number'),
          'step_up_grant_id': grant,
          'expected_version': version,
        });
      case 'brand':
        await _saved(
          version == 0 ? 'POST' : 'PATCH',
          version == 0
              ? '/api/v1/vendor/businesses/{businessId}/brands'
              : '/api/v1/vendor/businesses/{businessId}/brands/{brandId}',
          {
            'display_name': _text('display_name'),
            'description': _text('description'),
            if (_record['media_refs'] is List)
              'media_refs': _record['media_refs'],
            if (version != 0) 'expected_version': version,
          },
          params: {...params, if (version != 0) 'brandId': _record['id']},
        );
      case 'branch':
        final lat = num.parse(_text('latitude')),
            lng = num.parse(_text('longitude'));
        if (lat < -90 || lat > 90 || lng < -180 || lng > 180) {
          throw const FormatException('Enter valid latitude and longitude.');
        }
        await _saved(
          version == 0 ? 'POST' : 'PATCH',
          version == 0
              ? '/api/v1/vendor/businesses/{businessId}/branches'
              : '/api/v1/vendor/businesses/{businessId}/branches/{branchId}',
          {
            'display_name': _text('display_name'),
            'address': {
              'line1': _text('line1'),
              'city': _text('city'),
              'region': _text('region'),
              'country_code': _text('country_code').toUpperCase(),
              if (_text('line2').isNotEmpty) 'line2': _text('line2'),
              if (_text('postal_code').isNotEmpty)
                'postal_code': _text('postal_code'),
            },
            'latitude': lat,
            'longitude': lng,
            'timezone': _text('timezone'),
            if (_brand.isNotEmpty) 'brand_id': _brand,
            if (version != 0) 'expected_version': version,
          },
          params: {...params, if (version != 0) 'branchId': _record['id']},
        );
      case 'capabilities':
        if (_capability.isEmpty) {
          throw const FormatException('Choose a business type.');
        }
        await _saved(
          'POST',
          '/api/v1/vendor/businesses/{businessId}/capabilities',
          {'capability_type': _capability},
          params: params,
        );
    }
  });
  Future<String?> _securityGrant(String purpose) async {
    if (_challenge == null) {
      final challenge = await widget.runtime.auth.startStepUp(purpose);
      _challenge = challenge.challengeId;
      _developmentCode = challenge.debugOtp;
      if (_developmentCode?.isNotEmpty == true) {
        _field('security_code').text = _developmentCode!;
      } else if (_mock) {
        _field('security_code').text = '123456';
      }
      _challengeExpires = DateTime.now().add(
        Duration(seconds: challenge.expiresIn),
      );
      if (mounted) setState(() {});
      return null;
    }
    if (_challengeExpires == null ||
        DateTime.now().isAfter(_challengeExpires!)) {
      _challenge = null;
      _field('security_code').clear();
      throw const FormatException(
        'Your security code expired. Request a new code.',
      );
    }
    if (!RegExp(r'^\d{6}$').hasMatch(_text('security_code'))) {
      throw const FormatException('Enter the six-digit security code.');
    }
    final grant = await widget.runtime.auth.verifyStepUp(
      purpose: purpose,
      challengeId: _challenge!,
      otp: _text('security_code'),
    );
    _challenge = null;
    _field('security_code').clear();
    return grant.grantId;
  }

  Future<void> _open(String kind, {JsonObject? record}) async {
    await Navigator.push(
      context,
      setupRoute<void>(
        _SetupForm(
          controller: c,
          runtime: widget.runtime,
          kind: kind,
          record: record,
          onSettings: widget.onSettings,
        ),
      ),
    );
    await c.load();
    if (mounted) setState(() {});
  }

  Future<void> _branchSettings(JsonObject branch) async {
    await Navigator.push(
      context,
      MaterialPageRoute<void>(
        builder: (_) => BranchSetupScreen(
          controller: c,
          runtime: widget.runtime,
          branch: branch,
          onSettings: widget.onSettings,
        ),
      ),
    );
    await c.load();
    if (mounted) setState(() {});
  }

  List<JsonObject> get _subjects => [
    {'type': 'BUSINESS', 'id': c.businessId, 'name': c.business['legal_name']},
    if (c.representative.isNotEmpty)
      {
        'type': 'REPRESENTATIVE',
        'id': c.representative['id'],
        'name':
            '${c.representative['first_name']} ${c.representative['last_name']}',
      },
    for (final b in c.branches)
      {'type': 'BRANCH', 'id': b['id'], 'name': b['display_name']},
    for (final b in c.brands)
      {'type': 'BRAND', 'id': b['id'], 'name': b['display_name']},
    for (final branch in c.branches)
      for (final cap in BusinessOnboardingController.objects(
        branch['capabilities'],
      ))
        {
          'type': 'BRANCH_CAPABILITY',
          'id': cap['id'],
          'branch_id': branch['id'],
          'name':
              '${branch['display_name']} / ${_label(cap['capability_type'])}',
        },
    for (final b in c.capabilities)
      {
        'type': 'CAPABILITY',
        'id': b['id'],
        'name': _label(b['capability_type']),
      },
  ];
  JsonObject get _selectedSubject => _subjects.firstWhere(
    (s) => '${s['type']}:${s['id']}' == _subject,
    orElse: () => _subjects.first,
  );
  bool _matchesDocument(JsonObject d, JsonObject s) =>
      d['dimension'] == _dimension &&
      switch (s['type']) {
        'REPRESENTATIVE' => d['representative_id'] == s['id'],
        'BRANCH' => d['branch_id'] == s['id'],
        'BRAND' => d['brand_id'] == s['id'],
        'BRANCH_CAPABILITY' => d['branch_id'] == s['branch_id'],
        _ =>
          d['representative_id'] == null &&
              d['branch_id'] == null &&
              d['brand_id'] == null,
      };
  Future<void> _pick() => _task(() async {
    if (_mock) {
      await _demoDocument();
      return;
    }
    final result = await FilePicker.platform.pickFiles(
      type: FileType.custom,
      allowedExtensions: ['jpg', 'jpeg', 'png', 'webp', 'pdf'],
      withData: true,
      allowMultiple: false,
    );
    if (result == null) return;
    final file = result.files.single;
    if (file.bytes == null || file.size <= 0 || file.size > 10485760) {
      throw const FormatException('Choose a file smaller than 10 MB.');
    }
    final mime = switch (file.extension?.toLowerCase()) {
      'jpg' || 'jpeg' => 'image/jpeg',
      'png' => 'image/png',
      'webp' => 'image/webp',
      'pdf' => 'application/pdf',
      _ => null,
    };
    if (mime == null) {
      throw const FormatException('Choose a JPEG, PNG, WebP or PDF.');
    }
    c.beginNewUpload();
    _fileBytes = file.bytes;
    _fileName = file.name;
    _mime = mime;
    _pendingMedia = null;
  }, 'pick');
  Future<void> _upload() => _task(() async {
    if (!_form.currentState!.validate()) return;
    if (_fileBytes == null) {
      throw const FormatException('Choose your document first.');
    }
    final subject = _selectedSubject;
    if (_pendingMedia == null) {
      final grant = await c.request(
        'POST',
        '/api/v1/vendor/onboarding/document-uploads',
        body: {
          'dimension': _dimension,
          'subject_type': subject['type'],
          'subject_ref': subject['id'],
          'mime_type': _mime,
          'byte_size': _fileBytes!.length,
        },
      );
      try {
        await DocumentUploadClient().upload(
          grant.object,
          _fileBytes!,
          _mime!,
          transport: widget.runtime.mockUploadTransport,
        );
      } catch (_) {
        final receipt = await c.request(
          'GET',
          '/api/v1/vendor/onboarding/document-uploads/{mediaRef}',
          params: {'mediaRef': grant.object['media_ref'] as String},
        );
        if (receipt.object['upload_status'] != 'COMPLETED' ||
            [
              'UNSAFE',
              'QUARANTINED',
            ].contains(receipt.object['safety_status'])) {
          throw const FormatException(
            'Upload could not be confirmed. Choose the document again to start a new upload.',
          );
        }
      }
      _pendingMedia = grant.object['media_ref'] as String;
    }
    final attached = await c.request(
      _record['status'] == 'DRAFT' ? 'PATCH' : 'POST',
      _record['status'] == 'DRAFT'
          ? '/api/v1/vendor/onboarding/documents/{documentId}'
          : '/api/v1/vendor/onboarding/documents',
      params: _record['status'] == 'DRAFT'
          ? {'documentId': _record['id']}
          : const {},
      body: {
        'dimension': _dimension,
        'document_type': _text('document_type'),
        'media_ref': _pendingMedia,
        'expected_version': _record['status'] == 'DRAFT'
            ? _record['version']
            : 0,
        if (subject['type'] == 'REPRESENTATIVE')
          'representative_id': subject['id'],
        if (subject['type'] == 'BRANCH') 'branch_id': subject['id'],
        if (subject['type'] == 'BRANCH_CAPABILITY')
          'branch_id': subject['branch_id'],
        if (subject['type'] == 'BRAND') 'brand_id': subject['id'],
      },
    );
    if (attached.object['id'] != null) {
      _selectedDocs.add(attached.object['id'].toString());
    }
    _pendingMedia = null;
    _fileBytes = null;
    _fileName = null;
    await c.load();
    if (mounted) {
      if (_record['status'] == 'DRAFT') {
        Navigator.pop(context);
      } else {
        setState(() {});
      }
    }
  }, 'upload');
  Future<void> _deleteDocument(JsonObject doc) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Remove draft document?'),
        content: const Text(
          'You can upload a replacement whenever you are ready.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Keep document'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Remove'),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    await _task(() async {
      await c.request(
        'DELETE',
        '/api/v1/vendor/onboarding/documents/{documentId}',
        params: {'documentId': doc['id']},
        body: {'expected_version': doc['version']},
      );
      _selectedDocs.remove(doc['id']);
      await c.load();
      if (mounted) setState(() {});
    });
  }

  Future<void> _verify() => _task(() async {
    if (_selectedDocs.isEmpty) {
      throw const FormatException('Select the documents to submit.');
    }
    final subject = _selectedSubject;
    final existing = c.verifications.where(
      (v) =>
          v['dimension'] == _dimension &&
          v['subject_type'] == subject['type'] &&
          v['subject_ref'] == subject['id'],
    );
    final resubmit = existing.any(
      (v) => ['REJECTED', 'EXPIRED'].contains(v['status']),
    );
    await c.request(
      'POST',
      '/api/v1/vendor/onboarding/verifications/{dimension}/${resubmit ? 'resubmit' : 'submit'}',
      params: {'dimension': _dimension},
      body: {
        'subject_type': subject['type'],
        'subject_ref': subject['id'],
        'document_refs': _selectedDocs.toList(),
      },
    );
    _selectedDocs.clear();
    await c.load();
    if (!mounted) return;
    setState(() {});
    await _feedback(
      'Documents submitted',
      'Your ${_label(_dimension).toLowerCase()} documents are under review. You can continue with your remaining checks.',
    );
    if (!mounted) return;
    for (final requirement in c.requirements) {
      if (requirement['satisfied'] != true &&
          !reviewPending(requirement['status']) &&
          requirement['dimension'] != null &&
          requirement['dimension'] != '' &&
          _requirementSubject(requirement) != null) {
        _chooseRequirement(requirement);
        break;
      }
    }
  }, 'submit');
  @override
  Widget build(BuildContext context) => VendorScreenFrame(
    modeLabel: widget.runtime.mode.name,
    onSettings: widget.onSettings,
    onBack: _working ? null : () => Navigator.pop(context),
    leading: const Text(
      'BUSINESS SETUP',
      style: TextStyle(
        fontSize: 11,
        letterSpacing: 1.5,
        fontWeight: FontWeight.w700,
      ),
    ),
    footer: widget.kind == 'locations'
        ? Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (!c.branches.any((b) => b['status'] != 'CLOSED')) ...[
                Text(
                  'Add at least one branch to continue.',
                  textAlign: TextAlign.center,
                  style: TextStyle(
                    fontSize: 12,
                    color: KarryamColors.indigo.forInk(context),
                  ),
                ),
                const SizedBox(height: 10),
              ],
              PrimaryAction(
                label: 'Continue to documents',
                busy: _working,
                onPressed:
                    _working || !c.branches.any((b) => b['status'] != 'CLOSED')
                    ? null
                    : () => _task(_advance),
              ),
            ],
          )
        : null,
    body: SetupFormSurface(
      child: Column(
        children: [
          if (widget.kind != 'bank')
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 10, 24, 6),
              child: SetupStepIndicator(
                current: switch (widget.kind) {
                  'business' => 0,
                  'representative' => 1,
                  'capabilities' => 2,
                  'locations' || 'brand' || 'branch' => 3,
                  _ => 4,
                },
                completed: [
                  for (final kind in [
                    'business',
                    'representative',
                    'capabilities',
                    'locations',
                    'verification',
                  ])
                    stepProgress(c, kind).state == SetupState.complete,
                ],
              ),
            ),
          Expanded(
            child: Form(
              key: _form,
              child: ListView(
                padding: const EdgeInsets.fromLTRB(24, 20, 24, 28),
                children: [
                  SetupChapterHeader(kind: widget.kind),
                  const SizedBox(height: 28),
                  if (_error != null || c.error != null) ...[
                    InlineNotice(message: _error ?? c.error!, error: true),
                    const SizedBox(height: 18),
                  ],
                  ..._content(),
                  const SizedBox(height: 24),
                  if (!['locations', 'verification'].contains(widget.kind))
                    PrimaryAction(
                      label: _challenge != null
                          ? 'Verify & save'
                          : widget.kind == 'bank' ||
                                (widget.kind == 'representative' &&
                                    c.verifications.any(
                                      (v) =>
                                          v['dimension'] == 'IDENTITY' &&
                                          v['status'] == 'VERIFIED',
                                    ))
                          ? 'Continue securely'
                          : 'Save & continue',
                      busy: _working,
                      onPressed: _working ? null : _save,
                    ),
                ],
              ),
            ),
          ),
        ],
      ),
    ),
  );
  List<Widget> _content() {
    final address = _record['address'] is Map
        ? jsonObject(_record['address'])
        : <String, dynamic>{};
    return switch (widget.kind) {
      'business' => [
        const SetupSectionLabel(
          label: 'BUSINESS IDENTITY',
          caption: 'A name to build on.',
        ),
        _input('legal_name', 'Legal business name', min: 2, max: 200),
        _input('trading_name', 'Trading name', required: false, max: 200),
        const SetupSectionLabel(
          label: 'CONTACT',
          caption: 'Keep the conversation open.',
        ),
        _input('support_email', 'Support email', required: false, email: true),
        _input('support_phone', 'Support phone', required: false),
        // Karryam operates in Nigeria only: naira and Lagos time are fixed,
        // so they are sent without asking the user to type codes.
        Offstage(
          child: Column(
            children: [
              _input('currency', 'Currency', min: 3, max: 3),
              _input('timezone', 'Timezone'),
            ],
          ),
        ),
        const Text(
          'Prices are in Nigerian Naira (₦) and times use Lagos time (WAT).',
          style: TextStyle(fontSize: 12, height: 1.6),
        ),
      ],
      'representative' => [
        const SetupSectionLabel(
          label: 'REPRESENTATIVE',
          caption: 'Who is representing you?',
        ),
        DropdownButtonFormField<String>(
          isExpanded: true,
          initialValue: _capacity,
          decoration: const InputDecoration(labelText: 'Role'),
          items: [
            for (final v in ['OWNER', 'DIRECTOR', 'AUTHORIZED_SIGNATORY'])
              DropdownMenuItem(
                value: v,
                child: Text(
                  _label(v),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
          ],
          onChanged: _working ? null : (v) => setState(() => _capacity = v!),
        ),
        const SizedBox(height: 18),
        _input('first_name', 'First name', max: 100),
        _input('last_name', 'Last name', max: 100),
        const SetupSectionLabel(
          label: 'CONTACT DETAILS',
          caption: 'Stay within reach.',
        ),
        _input('email', 'Email', email: true),
        _input('phone', 'Phone', min: 7, max: 32),
        _input(
          'ownership_percent',
          'Ownership percentage (optional)',
          required: false,
          number: true,
        ),
        SwitchListTile.adaptive(
          contentPadding: EdgeInsets.zero,
          title: const Text('Authorized to represent this business'),
          value: _authorized,
          onChanged: _working ? null : (v) => setState(() => _authorized = v),
        ),
        if (_challenge != null) ...[
          InlineNotice(
            message: _developmentCode?.isNotEmpty == true
                ? 'Local development code filled in. Continue to verify.'
                : 'Enter the security code sent to your registered contact.',
          ),
          const SizedBox(height: 16),
          _input(
            'security_code',
            'Security code',
            number: true,
            min: 6,
            max: 6,
          ),
        ],
      ],
      'bank' => [
        if (c.bank.isNotEmpty) ...[
          InlineNotice(
            message:
                '${c.bank['resolved_account_name']}\n${c.bank['masked_account']} / ${_label(c.bank['status'])}',
          ),
          const SizedBox(height: 18),
        ],
        const SetupSectionLabel(
          label: 'ACCOUNT DETAILS',
          caption: 'Your business banking.',
        ),
        _input('bank_code', 'Bank code', min: 2),
        _input(
          'account_number',
          'Account number',
          number: true,
          min: 6,
          secret: true,
        ),
        if (_challenge != null) ...[
          InlineNotice(
            message: _developmentCode?.isNotEmpty == true
                ? 'Local development code filled in. Continue to verify.'
                : 'Enter the security code sent to your registered contact.',
          ),
          const SizedBox(height: 16),
          _input(
            'security_code',
            'Security code',
            number: true,
            min: 6,
            max: 6,
          ),
          TextButton(
            onPressed: _working
                ? null
                : () => _task(() async {
                    _challenge = null;
                    _developmentCode = null;
                    _field('security_code').clear();
                    await _securityGrant('finance.bank_change');
                  }, 'security'),
            child: const Text('Request a new code'),
          ),
        ],
      ],
      'brand' => [
        _input('display_name', 'Brand name', min: 2, max: 160),
        _input('description', 'Description', required: false, max: 2000),
      ],
      'branch' => [
        _input('display_name', 'Branch name', min: 2, max: 160),
        DropdownButtonFormField<String>(
          isExpanded: true,
          initialValue: _brand,
          decoration: const InputDecoration(labelText: 'Brand (optional)'),
          items: [
            const DropdownMenuItem(value: '', child: Text('No brand')),
            for (final b in c.brands)
              DropdownMenuItem(
                value: b['id'] as String,
                child: Text(
                  b['display_name'].toString(),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
          ],
          onChanged: _working ? null : (v) => setState(() => _brand = v!),
        ),
        const SizedBox(height: 18),
        Offstage(
          child: _input(
            'country_code',
            'Country',
            initial: address['country_code'],
            min: 2,
            max: 2,
          ),
        ),
        for (final entry in {
          'line1': 'Address line 1',
          'line2': 'Address line 2 (optional)',
          'city': 'City or town',
          'region': '',
          'postal_code': 'Postal code (optional)',
        }.entries)
          if (entry.key == 'region')
            _stateField(address['region'])
          else
            _input(
              entry.key,
              entry.value,
              initial: address[entry.key],
              required: !['line2', 'postal_code'].contains(entry.key),
              min: switch (entry.key) {
                'country_code' || 'city' || 'region' => 2,
                'line1' => 3,
                _ => null,
              },
              max: switch (entry.key) {
                'country_code' => 2,
                'line1' => 240,
                _ => null,
              },
            ),
        FormField<LatLng>(
          initialValue:
              double.tryParse(_field('latitude').text) != null &&
                  double.tryParse(_field('longitude').text) != null
              ? LatLng(
                  double.parse(_field('latitude').text),
                  double.parse(_field('longitude').text),
                )
              : null,
          validator: (value) =>
              value == null ? 'Choose your branch location on the map.' : null,
          builder: (field) => Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              ListTile(
                contentPadding: EdgeInsets.zero,
                leading: const Icon(Icons.location_on_outlined),
                title: Text(
                  field.value == null
                      ? 'Pin your branch location'
                      : 'Branch location selected',
                ),
                subtitle: Text(
                  field.value == null
                      ? 'Search an address, choose on the map or use your current location'
                      : 'Pinned on the map. Tap to change it.',
                ),
                trailing: const Icon(Icons.chevron_right_rounded),
                onTap: _working
                    ? null
                    : () async {
                        final selected = await Navigator.of(context)
                            .push<LatLng>(
                              setupRoute<LatLng>(
                                BranchLocationPicker(initial: field.value),
                              ),
                            );
                        if (!mounted || selected == null) return;
                        _field('latitude').text = selected.latitude.toString();
                        _field('longitude').text = selected.longitude
                            .toString();
                        field.didChange(selected);
                      },
              ),
              if (field.hasError)
                Text(
                  field.errorText!,
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
            ],
          ),
        ),
        Offstage(
          child: _input(
            'timezone',
            'Timezone',
            initial: _record['timezone'] ?? c.business['timezone'],
          ),
        ),
      ],
      'capabilities' => [
        for (final cap in c.capabilities)
          ListTile(
            contentPadding: EdgeInsets.zero,
            leading: Icon(
              Icons.check_circle,
              color: KarryamColors.tealInk.forInk(context),
            ),
            title: Text(_label(cap['capability_type'])),
            subtitle: Text(_label(cap['status'])),
          ),
        DropdownButtonFormField<String>(
          key: ValueKey(
            c.capabilities.map((cap) => cap['capability_type']).join(','),
          ),
          initialValue:
              _capability.isEmpty ||
                  c.capabilities.any(
                    (cap) => cap['capability_type'] == _capability,
                  )
              ? null
              : _capability,
          decoration: const InputDecoration(labelText: 'Add a business type'),
          isExpanded: true,
          items: [
            for (final cap in c.catalogue.where(
              (v) =>
                  !c.capabilities.any((a) => a['capability_type'] == v['type']),
            ))
              DropdownMenuItem(
                value: cap['type'] as String,
                child: Text(_label(cap['type'])),
              ),
          ],
          onChanged: _working ? null : (v) => setState(() => _capability = v!),
        ),
      ],
      'locations' => [
        const Text(
          'Brands',
          style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700),
        ),
        for (final brand in c.brands)
          ListTile(
            contentPadding: EdgeInsets.zero,
            title: Text(brand['display_name'].toString()),
            trailing: const Icon(Icons.chevron_right),
            onTap: () => _open('brand', record: brand),
          ),
        const SizedBox(height: 12),
        OutlinedButton.icon(
          onPressed: () => _open('brand'),
          icon: const Icon(Icons.add),
          label: const Text('Add brand'),
        ),
        const SizedBox(height: 28),
        const Text(
          'Branches',
          style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700),
        ),
        for (final branch in c.branches)
          ListTile(
            contentPadding: EdgeInsets.zero,
            title: Text(branch['display_name'].toString()),
            subtitle: Text(_label(branch['status'])),
            trailing: IconButton(
              tooltip: 'Branch hours, service area and business types',
              onPressed: () => _branchSettings(branch),
              icon: const Icon(Icons.tune_rounded),
            ),
            onTap: () => _open('branch', record: branch),
          ),
        const SizedBox(height: 12),
        OutlinedButton.icon(
          onPressed: () => _open('branch'),
          icon: const Icon(Icons.add),
          label: const Text('Add branch'),
        ),
      ],
      'verification' => _verification(),
      _ => [],
    };
  }

  bool get _selectedCheckApproved => c.requirements.any(
    (r) =>
        r['dimension'] == _dimension &&
        (r['subject_ref'] == null ||
            r['subject_ref'] == '' ||
            r['subject_ref'] == _selectedSubject['id']) &&
        r['satisfied'] == true,
  );

  bool get _selectedCheckDone =>
      _selectedCheckApproved ||
      c.verifications.any(
        (v) =>
            v['dimension'] == _dimension &&
            v['subject_ref'] == _selectedSubject['id'] &&
            reviewPending(v['status']),
      );

  JsonObject? _requirementSubject(JsonObject r) {
    final type = switch (r['dimension']) {
      'IDENTITY' => 'REPRESENTATIVE',
      'LOCATION' => 'BRANCH',
      'BRAND' => 'BRAND',
      'FOOD_KITCHEN' => 'BRANCH_CAPABILITY',
      _ =>
        r['scope'] == 'BRANCH_CAPABILITY'
            ? 'BRANCH_CAPABILITY'
            : r['scope'] == 'CAPABILITY'
            ? 'CAPABILITY'
            : 'BUSINESS',
    };
    for (final subject in _subjects) {
      if (subject['type'] == type &&
          (r['subject_ref'] == null ||
              r['subject_ref'] == '' ||
              r['subject_ref'] == subject['id'])) {
        return subject;
      }
    }
    return null;
  }

  void _chooseRequirement(JsonObject r) {
    final subject = _requirementSubject(r);
    if (subject == null || r['dimension'] == null || r['dimension'] == '') {
      return;
    }
    setState(() {
      _dimension = r['dimension'].toString();
      _subject = '${subject['type']}:${subject['id']}';
      _selectedDocs.clear();
      _fileBytes = null;
      _fileName = null;
      _fields['document_type']?.clear();
    });
    if (_mock) unawaited(_demoDocument());
  }

  Widget _requirementCard(JsonObject r) {
    final done = r['satisfied'] == true;
    final pending = reviewPending(r['status']);
    final attention = needsAttention(r['status']);
    final subject = _requirementSubject(r);
    final documentCheck = r['dimension'] != null && r['dimension'] != '';
    final selected =
        documentCheck &&
        r['dimension'] == _dimension &&
        subject?['id'] == _selectedSubject['id'];
    final color =
        (done
                ? const Color(0xFF16836B)
                : attention
                ? const Color(0xFFC45145)
                : pending
                ? const Color(0xFFAE781E)
                : KarryamColors.gray)
            .forInk(context);
    return Container(
      margin: const EdgeInsets.only(bottom: 12),
      decoration: BoxDecoration(
        color: Colors.white.forSurface(context),
        borderRadius: BorderRadius.circular(20),
        border: Border.all(
          color: selected
              ? KarryamColors.teal.forOutline(context)
              : KarryamColors.gray.forOutline(context).withValues(alpha: .18),
        ),
      ),
      child: Material(
        type: MaterialType.transparency,
        child: ListTile(
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(20),
          ),
          contentPadding: const EdgeInsets.symmetric(
            horizontal: 16,
            vertical: 10,
          ),
          leading: Container(
            width: 44,
            height: 44,
            decoration: BoxDecoration(
              color: color.withValues(alpha: .10),
              borderRadius: BorderRadius.circular(14),
            ),
            child: Icon(
              done
                  ? Icons.check_circle_outline_rounded
                  : pending
                  ? Icons.hourglass_top_rounded
                  : switch (r['dimension']) {
                      'IDENTITY' => Icons.badge_outlined,
                      'BUSINESS' => Icons.business_outlined,
                      'LOCATION' => Icons.location_on_outlined,
                      'BANK' => Icons.account_balance_outlined,
                      'FOOD_KITCHEN' => Icons.restaurant_outlined,
                      _ => Icons.fact_check_outlined,
                    },
              color: color,
            ),
          ),
          title: Text(
            _label(documentCheck ? r['dimension'] : r['code']),
            style: const TextStyle(fontWeight: FontWeight.w700),
          ),
          subtitle: Padding(
            padding: const EdgeInsets.only(top: 5),
            child: Text(
              [
                if (documentCheck && subject != null)
                  subject['name']?.toString() ?? '',
                done
                    ? (documentCheck ? 'Approved' : 'Complete')
                    : pending
                    ? 'Under review'
                    : attention
                    ? 'Needs attention - replace and resubmit'
                    : documentCheck
                    ? 'Document required - tap to continue'
                    : 'Details required - tap to complete',
              ].where((v) => v.isNotEmpty).join('\n'),
              style: TextStyle(color: color, height: 1.5),
            ),
          ),
          trailing: done || pending
              ? null
              : Icon(Icons.arrow_forward_rounded, size: 20, color: color),
          onTap: _working || _pendingMedia != null || done || pending
              ? null
              : () {
                  if (documentCheck && subject != null) {
                    _chooseRequirement(r);
                  } else {
                    _open(requirementStep(r));
                  }
                },
        ),
      ),
    );
  }

  Future<void> _feedback(String title, String message) =>
      _showSetupFeedback(context, title, message);

  Future<void> _finishReview() => _task(() async {
    await c.request(
      'POST',
      '/api/v1/vendor/onboarding/${c.readiness['ready'] == true ? 'request-activation' : 'submit'}',
    );
    await c.load();
    if (!mounted) return;
    await _feedback(
      c.activated ? 'Your business is ready' : 'Business submitted',
      c.activated
          ? 'Your setup is complete.'
          : 'Your business has been submitted for review. You can return here to track each check.',
    );
    if (mounted) Navigator.pop(context);
  }, 'finish');

  List<Widget> _verification() {
    final subject = _selectedSubject;
    final docs = c.documents.where((d) => _matchesDocument(d, subject));
    return [
      const SetupSectionLabel(
        label: 'REQUIRED CHECKS',
        caption:
            'Choose a check, attach its documents, then submit for review.',
      ),
      for (final requirement in c.requirements) _requirementCard(requirement),
      const SizedBox(height: 20),
      if (_selectedCheckDone) ...[
        InlineNotice(
          message:
              'This check is ${_selectedCheckApproved ? 'approved' : 'under review'}. You do not need to submit it again.',
        ),
      ] else ...[
        Text(
          '${_label(_dimension)} documents',
          style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w700),
        ),
        const SizedBox(height: 6),
        Text(
          subject['name']?.toString() ?? '',
          style: TextStyle(color: KarryamColors.gray.forInk(context)),
        ),
        const SizedBox(height: 20),
        _documentTypeField(),
        SetupDocumentPicker(
          fileName: _fileName,
          onPressed: _working ? null : _pick,
        ),
        // Repeat the error here: the page header is off-screen while the
        // merchant is at the upload button, so a failure looked silent.
        if (_error != null && ['pick', 'upload'].contains(_errorAction)) ...[
          const SizedBox(height: 14),
          InlineNotice(message: _error!, error: true),
        ],
        const SizedBox(height: 18),
        PrimaryAction(
          label: _pendingMedia != null
              ? 'Finish attaching document'
              : 'Upload document',
          busy: _activeAction == 'upload',
          onPressed: _working || _fileBytes == null ? null : _upload,
        ),
        const SizedBox(height: 24),
        for (final doc in docs)
          CheckboxListTile(
            contentPadding: EdgeInsets.zero,
            title: Text(_label(doc['document_type'])),
            subtitle: Text(
              _label(doc['status']),
              style: TextStyle(
                color: (verificationColor(doc['status'])).forInk(context),
              ),
            ),
            value: _selectedDocs.contains(doc['id']),
            onChanged: _working
                ? null
                : (v) => setState(() {
                    if (v == true) {
                      _selectedDocs.add(doc['id']);
                    } else {
                      _selectedDocs.remove(doc['id']);
                    }
                  }),
          ),
        for (final doc in docs.where((d) => d['status'] == 'DRAFT'))
          Row(
            children: [
              Expanded(
                child: Text(
                  _label(doc['document_type']),
                  style: TextStyle(
                    fontSize: 12,
                    color: KarryamColors.gray.forInk(context),
                  ),
                ),
              ),
              IconButton(
                tooltip: 'Replace draft document',
                onPressed: _working
                    ? null
                    : () => _open('verification', record: doc),
                icon: const Icon(Icons.edit_outlined),
              ),
              IconButton(
                tooltip: 'Remove draft document',
                onPressed: _working ? null : () => _deleteDocument(doc),
                icon: const Icon(Icons.delete_outline_rounded),
              ),
            ],
          ),
        PrimaryAction(
          label: 'Submit for review',
          busy: _activeAction == 'submit',
          onPressed: _working || _selectedDocs.isEmpty ? null : _verify,
        ),
      ],
      const SizedBox(height: 24),
      PrimaryAction(
        label: c.readiness['ready'] == true
            ? 'Request activation'
            : c.submitted
            ? 'Business under review'
            : 'Submit business for review',
        busy: _activeAction == 'finish',
        onPressed: _working || !canFinishSetup(c) ? null : _finishReview,
      ),
      const SizedBox(height: 12),
      Text(
        c.submitted && c.readiness['ready'] != true
            ? 'Your business is under review. Our team checks every new '
                  'business by hand, usually within one working day. You can '
                  'set up your catalogue in the meantime.'
            : canFinishSetup(c)
            ? 'Your required checks are submitted or complete.'
            : 'Complete the outstanding checks above to submit your business.',
        textAlign: TextAlign.center,
        style: TextStyle(
          color: KarryamColors.gray.forInk(context),
          fontSize: 13,
        ),
      ),
    ];
  }
}
