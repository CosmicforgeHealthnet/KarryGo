import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:image_picker/image_picker.dart';

import '../../../core/theme/karryam_theme.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../../auth/models/provider_auth_models.dart';
import '../../auth/state/provider_auth_controller.dart';
import '../data/onboarding_api.dart';
import 'photo_field.dart';

/// New-driver setup: about you, licence and ID, your car, safety contacts and
/// a profile photo. Everything is saved together at the end, because the
/// first profile save is what marks the account as set up.
///
/// With [editing] it updates an existing account's details and documents
/// (no vehicle step) and pops when saved.
class OnboardingScreen extends StatefulWidget {
  const OnboardingScreen({
    super.key,
    required this.api,
    required this.auth,
    this.editing = false,
  });

  final OnboardingApi api;
  final ProviderAuthController auth;
  final bool editing;

  @override
  State<OnboardingScreen> createState() => _OnboardingScreenState();
}

class _OnboardingScreenState extends State<OnboardingScreen> {
  late final TaxiProvider? _p = widget.auth.state.provider;
  int _step = 0;
  bool _saving = false;
  String? _error;
  String _progress = '';

  // About you
  late final _first = TextEditingController(text: _p?.firstName);
  late final _last = TextEditingController(text: _p?.lastName);
  late final _email = TextEditingController(text: _p?.email);
  late final _phone = TextEditingController(text: _p?.phone);
  late final _state = TextEditingController(text: _p?.locationState);
  late final _city = TextEditingController(text: _p?.locationCity);
  // Licence and ID
  late final _licence = TextEditingController(text: _p?.driverLicenseNumber);
  late final _expiry = TextEditingController(text: _p?.licenseExpiryDate);
  XFile? _licencePhoto, _idPhoto, _regPhoto, _profilePhoto;
  // Car
  List<VehicleClassOption> _classes = const [];
  VehicleClassOption? _class;
  final _plate = TextEditingController();
  final _make = TextEditingController();
  final _model = TextEditingController();
  final _color = TextEditingController();
  final _year = TextEditingController();
  // Safety
  late final _gName = TextEditingController(text: _p?.guarantorName);
  late final _gPhone = TextEditingController(text: _p?.guarantorPhone);
  late final _eName = TextEditingController(text: _p?.emergencyContactName);
  late final _ePhone = TextEditingController(text: _p?.emergencyContactPhone);
  late final _eRelation = TextEditingController(
    text: _p?.emergencyContactRelationship,
  );

  List<String> get _titles => [
    'About you',
    'Licence and ID',
    if (!widget.editing) 'Your car',
    'Safety contacts',
    'Profile photo',
  ];

  String get _current => _titles[_step];

  @override
  void initState() {
    super.initState();
    for (final c in [
      _first,
      _last,
      _email,
      _phone,
      _state,
      _city,
      _licence,
      _expiry,
      _plate,
      _gName,
      _gPhone,
      _eName,
      _ePhone,
      _eRelation,
    ]) {
      c.addListener(() => setState(() {}));
    }
    if (!widget.editing) {
      widget.api.vehicleClasses().then((classes) {
        if (mounted) setState(() => _classes = classes);
      }, onError: (_) {});
    }
  }

  @override
  void dispose() {
    for (final c in [
      _first,
      _last,
      _email,
      _phone,
      _state,
      _city,
      _licence,
      _expiry,
      _plate,
      _make,
      _model,
      _color,
      _year,
      _gName,
      _gPhone,
      _eName,
      _ePhone,
      _eRelation,
    ]) {
      c.dispose();
    }
    super.dispose();
  }

  /// What is still missing on the current step, or null when it is complete.
  String? get _missing {
    String? need(bool ok, String what) => ok ? null : what;
    bool filled(TextEditingController c) => c.text.trim().isNotEmpty;
    return switch (_current) {
      'About you' =>
        need(
              filled(_first) && filled(_last),
              'Enter your first and last name.',
            ) ??
            need(
              filled(_state) && filled(_city),
              'Enter the state and city you drive in.',
            ) ??
            need(
              filled(_email) || filled(_phone),
              'Add a phone number or email.',
            ),
      'Licence and ID' =>
        need(
              filled(_licence) && filled(_expiry),
              'Enter your licence number and expiry date.',
            ) ??
            need(
              _licencePhoto != null || (_p?.driverLicenseUrl ?? '').isNotEmpty,
              'Add a photo of your driver licence.',
            ) ??
            need(
              _idPhoto != null || (_p?.govIdUrl ?? '').isNotEmpty,
              'Add a photo of your government ID.',
            ),
      'Your car' =>
        need(_class != null, 'Choose your vehicle class.') ??
            need(filled(_plate), 'Enter the plate number.') ??
            need(
              _regPhoto != null || (_p?.vehicleRegUrl ?? '').isNotEmpty,
              'Add a photo of the vehicle registration.',
            ),
      'Safety contacts' =>
        need(
              filled(_gName) && filled(_gPhone),
              'Add a guarantor and their phone number.',
            ) ??
            need(
              filled(_eName) && filled(_ePhone),
              'Add an emergency contact and their phone number.',
            ),
      _ => null,
    };
  }

  Future<void> _pickExpiry() async {
    final now = DateTime.now();
    final picked = await showDatePicker(
      context: context,
      firstDate: now,
      lastDate: DateTime(now.year + 15),
      initialDate:
          DateTime.tryParse(_expiry.text) ?? now.add(const Duration(days: 365)),
      helpText: 'Licence expiry date',
    );
    if (picked != null) {
      _expiry.text =
          '${picked.year}-${picked.month.toString().padLeft(2, '0')}-${picked.day.toString().padLeft(2, '0')}';
    }
  }

  Future<void> _next() async {
    if (_step < _titles.length - 1) {
      setState(() => _step++);
      return;
    }
    await _finish();
  }

  Future<void> _finish() async {
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      Future<String> upload(XFile? f, String purpose, String label) async {
        if (f == null) return '';
        setState(() => _progress = 'Uploading $label…');
        return (await widget.api.uploadPhoto(f, purpose)).url;
      }

      final licenceUrl = await upload(
        _licencePhoto,
        'document_file',
        'your licence',
      );
      final idUrl = await upload(_idPhoto, 'document_file', 'your ID');
      final regUrl = await upload(
        _regPhoto,
        'document_file',
        'the vehicle registration',
      );
      var photoId = '', photoUrl = '';
      if (_profilePhoto != null) {
        setState(() => _progress = 'Uploading your photo…');
        final photo = await widget.api.uploadPhoto(
          _profilePhoto!,
          'profile_photo',
        );
        photoId = photo.id;
        photoUrl = photo.url;
      }

      setState(() => _progress = 'Saving your details…');
      String t(TextEditingController c) => c.text.trim();
      final provider = await widget.api.saveProfile({
        'first_name': t(_first),
        'last_name': t(_last),
        if (t(_email) != (_p?.email ?? '')) 'email': t(_email),
        if (t(_phone) != (_p?.phone ?? '')) 'phone': t(_phone),
        'location_state': t(_state),
        'location_city': t(_city),
        'service_type': 'taxi',
        'driver_license_number': t(_licence),
        'license_expiry_date': t(_expiry),
        'driver_license_url': licenceUrl,
        'gov_id_url': idUrl,
        'vehicle_reg_url': regUrl,
        'guarantor_name': t(_gName),
        'guarantor_phone': t(_gPhone),
        'emergency_contact_name': t(_eName),
        'emergency_contact_phone': t(_ePhone),
        'emergency_contact_relationship': t(_eRelation),
        'profile_photo_url': photoUrl,
        'photo_asset_id': photoId,
      });

      if (!widget.editing && _class != null) {
        setState(() => _progress = 'Adding your car…');
        await widget.api.addVehicle(
          vehicleClass: _class!.value,
          plateNumber: t(_plate).toUpperCase(),
          seats: _class!.maxPassengers,
          make: t(_make),
          model: t(_model),
          color: t(_color),
          year: int.tryParse(t(_year)),
        );
      }
      // Replacing the session's driver ends onboarding: the app moves on to
      // the home screen by itself.
      widget.auth.applyProviderUpdate(provider);
      if (widget.editing && mounted) Navigator.of(context).pop(true);
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    } finally {
      if (mounted) {
        setState(() {
          _saving = false;
          _progress = '';
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    final missing = _missing;
    final last = _step == _titles.length - 1;
    return PopScope(
      canPop: _step == 0,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop && !_saving) setState(() => _step--);
      },
      child: Scaffold(
        backgroundColor: colors.surface,
        appBar: AppBar(
          backgroundColor: colors.surface,
          surfaceTintColor: Colors.transparent,
          automaticallyImplyLeading: widget.editing || _step > 0,
          leading: _step > 0
              ? IconButton(
                  icon: const Icon(Icons.arrow_back),
                  onPressed: _saving ? null : () => setState(() => _step--),
                )
              : null,
          title: Text(
            widget.editing ? 'Update your details' : 'Set up your account',
          ),
          actions: [
            if (!widget.editing && _step == 0)
              TextButton(
                onPressed: widget.auth.logout,
                child: const Text('Sign out'),
              ),
          ],
        ),
        body: SafeArea(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 20),
                child: Row(
                  children: [
                    for (var i = 0; i < _titles.length; i++)
                      Expanded(
                        child: Container(
                          height: 4,
                          margin: const EdgeInsets.symmetric(horizontal: 2),
                          decoration: BoxDecoration(
                            color: i <= _step ? colors.primary : colors.border,
                            borderRadius: BorderRadius.circular(2),
                          ),
                        ),
                      ),
                  ],
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 16, 20, 4),
                child: Text(
                  'Step ${_step + 1} of ${_titles.length}',
                  style: TextStyle(color: colors.textMuted, fontSize: 12),
                ),
              ),
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 20),
                child: Text(
                  _current,
                  style: const TextStyle(
                    fontSize: 24,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
              Expanded(
                child: ListView(
                  padding: const EdgeInsets.fromLTRB(20, 16, 20, 16),
                  children: _fields(),
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 0, 20, 12),
                child: Column(
                  children: [
                    if (_error != null)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 8),
                        child: Text(
                          _error!,
                          style: TextStyle(color: colors.danger),
                        ),
                      )
                    else if (_saving)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 8),
                        child: Text(
                          _progress,
                          style: TextStyle(color: colors.textMuted),
                        ),
                      )
                    else if (missing != null)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 8),
                        child: Text(
                          missing,
                          style: TextStyle(
                            color: colors.textMuted,
                            fontSize: 12,
                          ),
                        ),
                      ),
                    KarryamButton(
                      label: last
                          ? (widget.editing ? 'Save' : 'Finish')
                          : 'Continue',
                      busy: _saving,
                      onPressed: missing == null ? _next : null,
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  List<Widget> _fields() {
    final colors = KarryamColors.of(context);
    switch (_current) {
      case 'About you':
        return [
          KarryamField(
            controller: _first,
            label: 'First name',
            textCapitalization: TextCapitalization.words,
          ),
          KarryamField(
            controller: _last,
            label: 'Last name',
            textCapitalization: TextCapitalization.words,
          ),
          KarryamField(
            controller: _phone,
            label: 'Phone number',
            hint: '0803 123 4567',
            keyboardType: TextInputType.phone,
          ),
          KarryamField(
            controller: _email,
            label: 'Email address',
            keyboardType: TextInputType.emailAddress,
          ),
          KarryamField(
            controller: _state,
            label: 'State',
            hint: 'Lagos',
            textCapitalization: TextCapitalization.words,
          ),
          KarryamField(
            controller: _city,
            label: 'City',
            hint: 'Ikeja',
            textCapitalization: TextCapitalization.words,
          ),
        ];
      case 'Licence and ID':
        return [
          KarryamField(
            controller: _licence,
            label: 'Driver licence number',
            textCapitalization: TextCapitalization.characters,
          ),
          KarryamField(
            controller: _expiry,
            label: 'Licence expiry date',
            readOnly: true,
            onTap: _pickExpiry,
            suffix: const Icon(Icons.calendar_today_outlined),
          ),
          PhotoField(
            label: 'Driver licence',
            file: _licencePhoto,
            alreadyOnFile: (_p?.driverLicenseUrl ?? '').isNotEmpty,
            onPicked: (f) => setState(() => _licencePhoto = f),
          ),
          PhotoField(
            label: 'Government ID (NIN slip, passport or voter card)',
            file: _idPhoto,
            alreadyOnFile: (_p?.govIdUrl ?? '').isNotEmpty,
            onPicked: (f) => setState(() => _idPhoto = f),
          ),
        ];
      case 'Your car':
        return [
          DropdownButtonFormField<VehicleClassOption>(
            initialValue: _class,
            isExpanded: true,
            decoration: InputDecoration(
              labelText: 'Vehicle class',
              filled: true,
              fillColor: colors.card,
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(12),
              ),
            ),
            items: [
              for (final c in _classes)
                DropdownMenuItem(
                  value: c,
                  child: Text(
                    '${c.label} (up to ${c.maxPassengers} passengers)',
                  ),
                ),
            ],
            onChanged: (c) => setState(() => _class = c),
          ),
          const SizedBox(height: 14),
          KarryamField(
            controller: _plate,
            label: 'Plate number',
            hint: 'LND 123 AB',
            textCapitalization: TextCapitalization.characters,
          ),
          KarryamField(
            controller: _make,
            label: 'Make (optional)',
            hint: 'Toyota',
            textCapitalization: TextCapitalization.words,
          ),
          KarryamField(
            controller: _model,
            label: 'Model (optional)',
            hint: 'Corolla',
            textCapitalization: TextCapitalization.words,
          ),
          KarryamField(
            controller: _color,
            label: 'Colour (optional)',
            textCapitalization: TextCapitalization.words,
          ),
          TextField(
            controller: _year,
            keyboardType: TextInputType.number,
            maxLength: 4,
            inputFormatters: [FilteringTextInputFormatter.digitsOnly],
            decoration: InputDecoration(
              labelText: 'Year (optional)',
              filled: true,
              fillColor: colors.card,
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(12),
              ),
            ),
          ),
          PhotoField(
            label: 'Vehicle registration papers',
            file: _regPhoto,
            alreadyOnFile: (_p?.vehicleRegUrl ?? '').isNotEmpty,
            onPicked: (f) => setState(() => _regPhoto = f),
          ),
        ];
      case 'Safety contacts':
        return [
          Text(
            'Guarantor',
            style: TextStyle(
              color: colors.textMuted,
              fontWeight: FontWeight.w600,
            ),
          ),
          const SizedBox(height: 8),
          KarryamField(
            controller: _gName,
            label: 'Full name',
            textCapitalization: TextCapitalization.words,
          ),
          KarryamField(
            controller: _gPhone,
            label: 'Phone number',
            keyboardType: TextInputType.phone,
          ),
          const SizedBox(height: 8),
          Text(
            'Emergency contact',
            style: TextStyle(
              color: colors.textMuted,
              fontWeight: FontWeight.w600,
            ),
          ),
          const SizedBox(height: 8),
          KarryamField(
            controller: _eName,
            label: 'Full name',
            textCapitalization: TextCapitalization.words,
          ),
          KarryamField(
            controller: _ePhone,
            label: 'Phone number',
            keyboardType: TextInputType.phone,
          ),
          KarryamField(
            controller: _eRelation,
            label: 'Relationship',
            hint: 'Sister',
            textCapitalization: TextCapitalization.words,
          ),
        ];
      default:
        return [
          Text(
            'Passengers see your photo when you arrive. Use a clear, recent photo of your face.',
            style: TextStyle(color: colors.textMuted),
          ),
          const SizedBox(height: 16),
          PhotoField(
            label: 'Profile photo (optional)',
            file: _profilePhoto,
            alreadyOnFile: (_p?.profilePhotoUrl ?? '').isNotEmpty,
            onPicked: (f) => setState(() => _profilePhoto = f),
          ),
        ];
    }
  }
}
