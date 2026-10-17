import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/config/nigerian_phone.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/widgets/vendor_widgets.dart';

class VendorSignInScreen extends StatefulWidget {
  const VendorSignInScreen({
    super.key,
    required this.runtime,
    required this.onSettings,
  });
  final VendorRuntime runtime;
  final VoidCallback onSettings;
  @override
  State<VendorSignInScreen> createState() => _VendorSignInScreenState();
}

class _VendorSignInScreenState extends State<VendorSignInScreen> {
  final _form = GlobalKey<FormState>();
  late final TextEditingController _phone, _email;
  late bool _useEmail;
  bool _registering = false;
  VendorRuntime get runtime => widget.runtime;
  @override
  void initState() {
    super.initState();
    _useEmail = runtime.useEmail;
    final draft = runtime.registrationDraft;
    _registering = draft != null;
    _phone = TextEditingController(
      text: !runtime.useEmail && runtime.identifier.isNotEmpty
          ? (normalizeNigerianPhone(runtime.identifier)?.substring(4) ??
                runtime.identifier)
          : runtime.mode == VendorDataMode.mock
          ? '8012345678'
          : '',
    );
    _email = TextEditingController(
      text: runtime.useEmail && runtime.identifier.isNotEmpty
          ? runtime.identifier
          : runtime.mode == VendorDataMode.mock
          ? 'owner@example.com'
          : '',
    );
  }

  @override
  void dispose() {
    _phone.dispose();
    _email.dispose();
    super.dispose();
  }

  void _submit() {
    if (!runtime.canRequest || !_form.currentState!.validate()) return;
    FocusScope.of(context).unfocus();
    runtime.start(
      _useEmail ? _email.text : normalizeNigerianPhone(_phone.text)!,
      email: _useEmail,
      registration: _registering ? <String, dynamic>{} : null,
    );
  }

  @override
  Widget build(BuildContext context) => VendorScreenFrame(
    modeLabel: runtime.mode.name,
    onSettings: runtime.busy ? null : widget.onSettings,
    onBack: null,
    body: SingleChildScrollView(
      padding: const EdgeInsets.fromLTRB(24, 22, 24, 16),
      child: Form(
        key: _form,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _SignInIntro(registering: _registering),
            const SizedBox(height: 12),
            Text(
              _registering
                  ? 'Verify your contact to create your vendor account.'
                  : 'Use your registered phone number or email.',
            ),
            const SizedBox(height: 22),
            Container(
              padding: const EdgeInsets.all(4),
              decoration: BoxDecoration(
                color: Color(0xFFE6ECE6).forSurface(context),
                borderRadius: BorderRadius.circular(14),
              ),
              child: Row(
                children: [
                  Expanded(
                    child: _MethodTab(
                      label: 'Phone number',
                      selected: !_useEmail,
                      onTap: runtime.busy
                          ? null
                          : () => setState(() => _useEmail = false),
                    ),
                  ),
                  Expanded(
                    child: _MethodTab(
                      label: 'Email address',
                      selected: _useEmail,
                      onTap: runtime.busy
                          ? null
                          : () => setState(() => _useEmail = true),
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 24),
            Text(
              _useEmail ? 'Email address' : 'Phone number',
              style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 10),
            TextFormField(
              key: ValueKey(_useEmail),
              controller: _useEmail ? _email : _phone,
              enabled: !runtime.busy,
              keyboardType: _useEmail
                  ? TextInputType.emailAddress
                  : TextInputType.phone,
              textInputAction: TextInputAction.done,
              autofillHints: [
                _useEmail ? AutofillHints.email : AutofillHints.telephoneNumber,
              ],
              autocorrect: false,
              decoration: InputDecoration(
                hintText: _useEmail ? 'you@yourbusiness.com' : '801 234 5678',
                prefixIcon: _useEmail
                    ? null
                    : const Padding(
                        padding: EdgeInsets.symmetric(horizontal: 14),
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Text('\u{1F1F3}\u{1F1EC}', style: TextStyle(fontSize: 22)),
                            SizedBox(width: 8),
                            Text(
                              '+234',
                              style: TextStyle(fontWeight: FontWeight.w600),
                            ),
                            SizedBox(width: 8),
                          ],
                        ),
                      ),
              ),
              validator: (value) {
                final contact = value?.trim() ?? '';
                if (_useEmail) {
                  return RegExp(r'^[^\s@]+@[^\s@]+\.[^\s@]+$').hasMatch(contact)
                      ? null
                      : 'Enter a valid email address.';
                }
                return normalizeNigerianPhone(contact) != null
                    ? null
                    : 'Enter a valid Nigerian phone number.';
              },
              onFieldSubmitted: (_) => _submit(),
            ),
            const SizedBox(height: 10),
            Text(
              _useEmail
                  ? 'We will send you a verification code.'
                  : 'Enter your Nigerian number with or without the first zero.',
              style: const TextStyle(fontSize: 12, height: 1.6),
            ),
            const SizedBox(height: 22),

            if (runtime.error != null) ...[
              const SizedBox(height: 16),
              InlineNotice(
                message: runtime.error!,
                error: true,
                onRetry: runtime.canRetry ? runtime.retry : null,
              ),
            ],
          ],
        ),
      ),
    ),
    footer: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        PrimaryAction(
          key: const Key('send-code'),
          label: runtime.retrySeconds > 0
              ? 'Try again in ${runtime.retrySeconds}s'
              : runtime.busy
              ? 'Sending your code…'
              : _registering
              ? 'Create account'
              : 'Send verification code',
          busy: runtime.busy,
          onPressed: runtime.canRequest ? _submit : null,
        ),
        const SizedBox(height: 14),
        TextButton(
          onPressed: runtime.busy
              ? null
              : () => setState(() {
                  _registering = !_registering;
                  runtime.error = null;
                }),
          child: Text(
            _registering
                ? 'Already have an account? Sign in'
                : 'New to Karryam? Create account',
          ),
        ),
      ],
    ),
  );
}

class _MethodTab extends StatelessWidget {
  const _MethodTab({
    required this.label,
    required this.selected,
    required this.onTap,
  });
  final String label;
  final bool selected;
  final VoidCallback? onTap;
  @override
  Widget build(BuildContext context) => Semantics(
    selected: selected,
    child: Material(
      color: (selected ? Colors.white : Colors.transparent).forSurface(context),
      borderRadius: BorderRadius.circular(11),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(11),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 14),
          child: Text(
            label,
            textAlign: TextAlign.center,
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w700,
              color: (selected ? KarryamColors.tealInk : KarryamColors.indigo)
                  .forInk(context),
            ),
          ),
        ),
      ),
    ),
  );
}

class VendorCodeScreen extends StatefulWidget {
  const VendorCodeScreen({
    super.key,
    required this.runtime,
    required this.onSettings,
  });
  final VendorRuntime runtime;
  final VoidCallback onSettings;
  @override
  State<VendorCodeScreen> createState() => _VendorCodeScreenState();
}

class _VendorCodeScreenState extends State<VendorCodeScreen> {
  final _code = TextEditingController();
  final _focus = FocusNode();
  String? _challenge;
  String? _lastTestCode;
  String? get _testCode => widget.runtime.testVerificationCode;

  VendorRuntime get runtime => widget.runtime;
  @override
  void initState() {
    super.initState();
    _challenge = runtime.auth.challenge?.challengeId;
    _lastTestCode = _testCode;
    _code.text = _testCode ?? '';
    _code.addListener(_update);
    _focus.addListener(_update);
  }

  void _update() {
    if (mounted) setState(() {});
  }

  @override
  void didUpdateWidget(covariant VendorCodeScreen oldWidget) {
    super.didUpdateWidget(oldWidget);
    final challenge = runtime.auth.challenge?.challengeId;
    final testCode = _testCode;
    if (_challenge != challenge || _lastTestCode != testCode) {
      _challenge = challenge;
      _lastTestCode = testCode;
      _code.text = testCode ?? '';
    }
  }

  @override
  void dispose() {
    _code.removeListener(_update);
    _focus.removeListener(_update);
    _code.dispose();
    _focus.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (_code.text.length != 6 || !runtime.canRequest || runtime.codeExpired) {
      return;
    }
    FocusScope.of(context).unfocus();
    await runtime.verify(_code.text);
    if (runtime.page == VendorPage.signedIn) {
      TextInput.finishAutofillContext(shouldSave: false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final expired = runtime.codeExpired;
    final seconds = runtime.remainingSeconds;
    final timer =
        '${(seconds ~/ 60).toString().padLeft(2, '0')}:${(seconds % 60).toString().padLeft(2, '0')}';
    return VendorScreenFrame(
      modeLabel: runtime.mode.name,
      onSettings: runtime.busy ? null : widget.onSettings,
      onBack: runtime.busy ? null : runtime.editContact,
      body: SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(24, 28, 24, 16),
        child: AutofillGroup(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              AccessArtwork(
                expired
                    ? Icons.schedule_rounded
                    : runtime.useEmail
                    ? Icons.mail_outline_rounded
                    : Icons.lock_outline_rounded,
              ),
              const SizedBox(height: 25),
              Text(
                expired
                    ? 'Your code\nhas expired.'
                    : _testCode != null
                    ? 'Verify your\nsign-in.'
                    : runtime.useEmail
                    ? 'Check your\nemail.'
                    : 'Check your\nmessages.',
                style: Theme.of(context).textTheme.headlineLarge,
              ),
              const SizedBox(height: 14),
              Text(
                _testCode != null
                    ? 'Test sign-in for'
                    : 'Enter the 6-digit code sent to',
              ),
              const SizedBox(height: 4),
              Text(
                runtime.maskedIdentifier,
                style: const TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w700,
                ),
              ),
              TextButton(
                onPressed: runtime.busy ? null : runtime.editContact,
                style: TextButton.styleFrom(padding: EdgeInsets.zero),
                child: Text(
                  runtime.useEmail
                      ? 'Change email address →'
                      : 'Change phone number →',
                ),
              ),
              const SizedBox(height: 18),
              const Text(
                'Verification code',
                style: TextStyle(fontSize: 13, fontWeight: FontWeight.w700),
              ),
              const SizedBox(height: 20),
              SizedBox(
                height: 58,
                child: Stack(
                  children: [
                    ExcludeSemantics(
                      child: Row(
                        children: List.generate(
                          6,
                          (i) => Expanded(
                            child: Container(
                              margin: EdgeInsets.only(right: i == 5 ? 0 : 8),
                              alignment: Alignment.center,
                              decoration: BoxDecoration(
                                color:
                                    (_focus.hasFocus && i == _code.text.length
                                            ? KarryamColors.mint
                                            : Colors.white)
                                        .forSurface(context),
                                borderRadius: BorderRadius.circular(12),
                                border: Border.all(
                                  color:
                                      (_focus.hasFocus && i == _code.text.length
                                              ? KarryamColors.tealInk
                                              : KarryamColors.border)
                                          .forOutline(context),
                                  width:
                                      _focus.hasFocus && i == _code.text.length
                                      ? 2
                                      : 1,
                                ),
                              ),
                              child: Text(
                                i < _code.text.length ? _code.text[i] : '',
                                style: const TextStyle(
                                  fontSize: 24,
                                  fontWeight: FontWeight.w700,
                                ),
                              ),
                            ),
                          ),
                        ),
                      ),
                    ),
                    Positioned.fill(
                      child: TextField(
                        key: const Key('otp-input'),
                        controller: _code,
                        focusNode: _focus,
                        enabled: !runtime.busy && !expired,
                        keyboardType: TextInputType.number,
                        textInputAction: TextInputAction.done,
                        autofillHints: const [AutofillHints.oneTimeCode],
                        inputFormatters: [
                          FilteringTextInputFormatter.digitsOnly,
                          LengthLimitingTextInputFormatter(6),
                        ],
                        style: TextStyle(
                          color: Colors.transparent.forInk(context),
                          fontSize: 24,
                        ),
                        cursorColor: Colors.transparent.forInk(context),
                        autocorrect: false,
                        enableSuggestions: false,
                        decoration: InputDecoration(
                          labelText: '6-digit verification code',
                          floatingLabelBehavior: FloatingLabelBehavior.never,
                          labelStyle: TextStyle(
                            color: Colors.transparent.forInk(context),
                          ),
                          filled: false,
                          border: InputBorder.none,
                          enabledBorder: InputBorder.none,
                          focusedBorder: InputBorder.none,
                          disabledBorder: InputBorder.none,
                        ),
                        onSubmitted: (_) => _submit(),
                      ),
                    ),
                  ],
                ),
              ),
              if (runtime.error != null) ...[
                const SizedBox(height: 16),
                InlineNotice(
                  message: runtime.error!,
                  error: true,
                  onRetry: runtime.canRetry ? runtime.retry : null,
                ),
              ],
              if (expired) ...[
                const SizedBox(height: 16),
                const InlineNotice(
                  message: 'Request a new code to continue signing in.',
                  error: true,
                ),
              ],
              const SizedBox(height: 12),
              Row(
                children: [
                  Expanded(
                    child: Text(
                      expired ? 'Code expired' : 'Code expires in $timer',
                      style: const TextStyle(fontSize: 12),
                    ),
                  ),
                  TextButton(
                    onPressed: runtime.canRequest ? runtime.resend : null,
                    child: Text(
                      runtime.retrySeconds > 0
                          ? 'Wait ${runtime.retrySeconds}s'
                          : 'Resend code',
                    ),
                  ),
                ],
              ),
              if (_testCode != null && !expired)
                Padding(
                  padding: const EdgeInsets.only(top: 12),
                  child: InlineNotice(
                    message:
                        'Test code: $_testCode. Filled in for staging testing.',
                  ),
                ),
              const SizedBox(height: 20),
              const Row(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Icon(Icons.lock_outline_rounded, size: 14),
                  SizedBox(width: 6),
                  Flexible(
                    child: Text(
                      'Keep your sign-in code private.',
                      style: TextStyle(fontSize: 11),
                    ),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
      footer: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          PrimaryAction(
            key: const Key('verify-code'),
            label: runtime.busy
                ? 'Verifying your code…'
                : expired
                ? 'Send a new code'
                : 'Verify and continue',
            busy: runtime.busy,
            onPressed: runtime.canRequest && (expired || _code.text.length == 6)
                ? expired
                      ? runtime.resend
                      : _submit
                : null,
          ),
          const SizedBox(height: 14),
          Text(
            _testCode != null
                ? 'Test code filled in. Tap Verify and continue.'
                : 'Use the latest code sent to you.\nYou can paste it directly into the code field.',
            textAlign: TextAlign.center,
            style: TextStyle(fontSize: 11, height: 1.6),
          ),
        ],
      ),
    );
  }
}

class VendorChoiceScreen extends StatelessWidget {
  const VendorChoiceScreen({
    super.key,
    required this.runtime,
    required this.onSettings,
  });
  final VendorRuntime runtime;
  final VoidCallback onSettings;
  @override
  Widget build(BuildContext context) => VendorScreenFrame(
    modeLabel: runtime.mode.name,
    onSettings: runtime.busy ? null : onSettings,
    onBack: runtime.busy ? null : runtime.editContact,
    body: SingleChildScrollView(
      padding: const EdgeInsets.fromLTRB(24, 28, 24, 16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const AccessArtwork(Icons.shopping_bag_outlined),
          const SizedBox(height: 25),
          Text(
            'Choose your\nvendor account.',
            style: Theme.of(context).textTheme.headlineLarge,
          ),
          const SizedBox(height: 14),
          const Text(
            'Your sign-in is linked to more than one vendor. Select the account to continue.',
          ),
          const SizedBox(height: 28),
          for (final id in runtime.auth.availableVendorIds)
            Padding(
              padding: const EdgeInsets.only(bottom: 12),
              child: Semantics(
                selected: runtime.selectedVendorId == id,
                button: true,
                child: Material(
                  color:
                      (runtime.selectedVendorId == id
                              ? KarryamColors.mint
                              : Colors.white)
                          .forSurface(context),
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(17),
                    side: BorderSide(
                      color:
                          (runtime.selectedVendorId == id
                                  ? KarryamColors.tealInk
                                  : KarryamColors.border)
                              .forOutline(context),
                      width: runtime.selectedVendorId == id ? 2 : 1,
                    ),
                  ),
                  child: InkWell(
                    onTap: runtime.busy ? null : () => runtime.selectVendor(id),
                    borderRadius: BorderRadius.circular(17),
                    child: Padding(
                      padding: const EdgeInsets.all(16),
                      child: Row(
                        children: [
                          const FeatureBadge(
                            Icons.shopping_bag_outlined,
                            size: 44,
                          ),
                          const SizedBox(width: 12),
                          Expanded(
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                const Text(
                                  'Vendor ID',
                                  style: TextStyle(
                                    fontSize: 13,
                                    fontWeight: FontWeight.w700,
                                  ),
                                ),
                                const SizedBox(height: 5),
                                Text(
                                  id.replaceAll('-', '-\u200B'),
                                  semanticsLabel: id,
                                  style: const TextStyle(
                                    fontSize: 11,
                                    letterSpacing: 0,
                                    height: 1.6,
                                  ),
                                ),
                              ],
                            ),
                          ),
                          const SizedBox(width: 8),
                          Icon(
                            runtime.selectedVendorId == id
                                ? Icons.radio_button_checked_rounded
                                : Icons.radio_button_off_rounded,
                            size: 20,
                            color:
                                (runtime.selectedVendorId == id
                                        ? KarryamColors.tealInk
                                        : KarryamColors.gray)
                                    .forInk(context),
                          ),
                        ],
                      ),
                    ),
                  ),
                ),
              ),
            ),
          const SizedBox(height: 12),
          const InlineNotice(
            message:
                'Continue with the same sign-in code.\nYou do not need to request another one.',
          ),
          if (runtime.error != null) ...[
            const SizedBox(height: 16),
            InlineNotice(
              message: runtime.error!,
              error: true,
              onRetry: runtime.canRetry ? runtime.retry : null,
            ),
          ],
        ],
      ),
    ),
    footer: PrimaryAction(
      label: runtime.busy ? 'Signing you in…' : 'Continue',
      busy: runtime.busy,
      onPressed:
          runtime.canRequest &&
              runtime.selectedVendorId != null &&
              !runtime.codeExpired
          ? runtime.chooseVendor
          : null,
    ),
  );
}

class VendorSessionScreen extends StatelessWidget {
  const VendorSessionScreen({
    super.key,
    required this.runtime,
    required this.onSettings,
    this.denied = false,
  });
  final VendorRuntime runtime;
  final VoidCallback onSettings;
  final bool denied;
  @override
  Widget build(BuildContext context) => VendorScreenFrame(
    modeLabel: runtime.mode.name,
    onSettings: runtime.busy ? null : onSettings,
    body: Center(
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(28),
        child: Column(
          children: [
            Container(
              width: 156,
              height: 156,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                border: Border.all(
                  color:
                      (denied
                              ? KarryamColors.error.withValues(alpha: .22)
                              : const Color(0xFFCCF3EE))
                          .forOutline(context),
                ),
              ),
              alignment: Alignment.center,
              child: AccessArtwork(
                denied
                    ? runtime.suspended
                          ? Icons.lock_outline_rounded
                          : Icons.person_outline_rounded
                    : Icons.check_rounded,
                size: 106,
                color: (denied ? KarryamColors.error : KarryamColors.teal)
                    .forInk(context),
              ),
            ),
            const SizedBox(height: 34),
            if (!denied) ...[
              Text(
                'YOU’RE CONNECTED',
                style: TextStyle(
                  fontSize: 11,
                  letterSpacing: 1.5,
                  fontWeight: FontWeight.w700,
                  color: KarryamColors.tealInk.forInk(context),
                ),
              ),
              const SizedBox(height: 18),
            ],
            Text(
              denied
                  ? runtime.suspended
                        ? 'Your access\nhas been suspended.'
                        : runtime.noBusinessAccess
                        ? 'No business linked\nto this account.'
                        : 'Sign-in not permitted.'
                  : 'Welcome back.',
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.headlineLarge,
            ),
            const SizedBox(height: 16),
            Text(
              denied
                  ? runtime.error ??
                        'Contact your business owner to check your access.'
                  : 'You’re signed in to your vendor account.',
              textAlign: TextAlign.center,
            ),
            if (!denied && runtime.mode == VendorDataMode.mock)
              const Padding(
                padding: EdgeInsets.only(top: 24),
                child: InlineNotice(
                  message: 'Mock session · No server request was made.',
                ),
              ),
          ],
        ),
      ),
    ),
    footer: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        if (denied)
          PrimaryAction(
            label: 'Use another account',
            onPressed: runtime.editContact,
          )
        else
          TextButton(
            onPressed: runtime.busy ? null : runtime.signOut,
            child: const Text('Sign out'),
          ),
        if (denied && runtime.noBusinessAccess)
          TextButton(
            onPressed: runtime.busy ? null : runtime.createAccount,
            child: const Text('Create a business account'),
          ),
        if (!denied)
          const Text(
            'Your business session is ready.',
            textAlign: TextAlign.center,
            style: TextStyle(fontSize: 12),
          ),
      ],
    ),
  );
}

class VendorConnectionScreen extends StatelessWidget {
  const VendorConnectionScreen({
    super.key,
    required this.runtime,
    required this.onSettings,
  });
  final VendorRuntime runtime;
  final VoidCallback onSettings;
  @override
  Widget build(BuildContext context) => VendorScreenFrame(
    modeLabel: runtime.mode.name,
    onSettings: runtime.busy ? null : onSettings,
    onBack: runtime.busy ? null : runtime.editContact,
    body: Center(
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(28),
        child: Column(
          children: [
            const AccessArtwork(Icons.wifi_off_rounded, size: 108),
            const SizedBox(height: 30),
            Text(
              'Let’s reconnect.',
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.headlineLarge,
            ),
            const SizedBox(height: 16),
            Text(
              runtime.error ??
                  'We couldn’t complete your request. Check your connection and try again.',
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    ),
    footer: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        PrimaryAction(
          label: 'Try again',
          busy: runtime.busy,
          onPressed: runtime.canRetry ? runtime.retry : null,
        ),
        const SizedBox(height: 14),
        const Text(
          'Your details are kept while you retry.',
          textAlign: TextAlign.center,
          style: TextStyle(fontSize: 12),
        ),
      ],
    ),
  );
}

class _SignInIntro extends StatelessWidget {
  const _SignInIntro({this.registering = false});
  final bool registering;
  @override
  Widget build(BuildContext context) => SizedBox(
    height: 170,
    child: Stack(
      children: [
        Positioned(
          right: 0,
          top: 0,
          child: Container(
            width: 94,
            height: 140,
            padding: const EdgeInsets.all(5),
            decoration: BoxDecoration(
              color: Colors.white.forSurface(context),
              borderRadius: BorderRadius.circular(60),
              boxShadow: [
                BoxShadow(
                  color: (KarryamColors.navy.withValues(
                    alpha: .07,
                  )).forInk(context),
                  blurRadius: 24,
                  offset: const Offset(0, 12),
                ),
              ],
            ),
            child: ClipRRect(
              borderRadius: BorderRadius.circular(60),
              child: Image.asset(
                'assets/onboarding/vendor-welcome.png',
                fit: BoxFit.cover,
                alignment: const Alignment(0, -.6),
                excludeFromSemantics: true,
              ),
            ),
          ),
        ),
        Positioned(
          right: 2,
          top: 117,
          child: Container(
            padding: const EdgeInsets.all(10),
            decoration: BoxDecoration(
              color: KarryamColors.teal.forSurface(context),
              shape: BoxShape.circle,
              border: Border.all(
                color: KarryamColors.canvas.forOutline(context),
                width: 4,
              ),
            ),
            child: const Icon(Icons.storefront_outlined, size: 19),
          ),
        ),
        Positioned(
          left: 0,
          top: 10,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                'VENDOR ACCOUNT',
                style: TextStyle(
                  fontSize: 9,
                  letterSpacing: 1.5,
                  fontWeight: FontWeight.w700,
                  color: KarryamColors.tealInk.forInk(context),
                ),
              ),
              const SizedBox(height: 20),
              Text.rich(
                TextSpan(
                  children: [
                    TextSpan(text: registering ? 'Join\n' : 'Sign in to\n'),
                    TextSpan(
                      text: 'Karryam.',
                      style: TextStyle(
                        color: KarryamColors.tealInk.forInk(context),
                        backgroundColor: KarryamColors.canvas.forSurface(
                          context,
                        ),
                      ),
                    ),
                  ],
                ),
                style: Theme.of(context).textTheme.headlineLarge!.copyWith(
                  fontSize: 40,
                  letterSpacing: -2,
                ),
              ),
            ],
          ),
        ),
      ],
    ),
  );
}
