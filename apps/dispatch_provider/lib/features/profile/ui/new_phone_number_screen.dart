import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'otp_confirmation_screen.dart';

class NewPhoneNumberScreen extends StatefulWidget {
  const NewPhoneNumberScreen({
    super.key,
    required this.countryFlag,
    required this.countryCode,
  });

  final String countryFlag;
  final String countryCode;

  @override
  State<NewPhoneNumberScreen> createState() => _NewPhoneNumberScreenState();
}

class _NewPhoneNumberScreenState extends State<NewPhoneNumberScreen> {
  final _numberController = TextEditingController();
  late final String _countryCode = widget.countryCode;
  late final String _countryFlag = widget.countryFlag;

  @override
  void dispose() {
    _numberController.dispose();
    super.dispose();
  }

  Future<void> _onVerify() async {
    final result = await Navigator.of(context).push<Map<String, String>>(
      MaterialPageRoute(
        builder: (_) => OtpConfirmationScreen(
          phoneDisplay: '$_countryFlag $_countryCode ${_numberController.text}',
          newCountryFlag: _countryFlag,
          newCountryCode: _countryCode,
          newPhoneNumber: _numberController.text,
        ),
      ),
    );

    if (!mounted) return;
    if (result != null) {
      Navigator.of(context).pop(result);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFFF7F7F7),
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 16, 24, 32),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              GestureDetector(
                onTap: () => Navigator.of(context).pop(),
                child: const Padding(
                  padding: EdgeInsets.only(top: 4, bottom: 4),
                  child: Align(
                    alignment: Alignment.centerLeft,
                    child: Icon(
                      Icons.arrow_back_ios_new,
                      size: 20,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                ),
              ),
              const SizedBox(height: 24),
              const Text(
                'New Phone Number',
                style: TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.w800,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 8),
              const Text(
                'Enter new phone number to proceed',
                style: TextStyle(
                  color: Color(0xFF888888),
                  fontSize: 13,
                  height: 1.5,
                ),
              ),
              const SizedBox(height: 36),
              const Text(
                'Enter your Phone Number',
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 12),
              Row(
                children: [
                  _CountryCodeBox(flag: _countryFlag, code: _countryCode),
                  const SizedBox(width: 10),
                  Expanded(
                    child: ValueListenableBuilder<TextEditingValue>(
                      valueListenable: _numberController,
                      builder: (context, value, _) {
                        return TextField(
                          controller: _numberController,
                          keyboardType: TextInputType.phone,
                          inputFormatters: [
                            FilteringTextInputFormatter.digitsOnly,
                          ],
                          style: const TextStyle(
                            fontSize: 14,
                            fontWeight: FontWeight.w600,
                            color: Color(0xFF4CAF50),
                          ),
                          decoration: InputDecoration(
                            hintText: 'Phone number',
                            hintStyle: const TextStyle(
                              color: Color(0xFFAAAAAA),
                              fontSize: 14,
                            ),
                            filled: true,
                            fillColor: Colors.white,
                            contentPadding: const EdgeInsets.symmetric(
                              horizontal: 16,
                              vertical: 14,
                            ),
                            border: OutlineInputBorder(
                              borderRadius: BorderRadius.circular(12),
                              borderSide: const BorderSide(
                                color: Color(0xFF4CAF50),
                                width: 1.5,
                              ),
                            ),
                            enabledBorder: OutlineInputBorder(
                              borderRadius: BorderRadius.circular(12),
                              borderSide: const BorderSide(
                                color: Color(0xFF4CAF50),
                                width: 1.5,
                              ),
                            ),
                            focusedBorder: OutlineInputBorder(
                              borderRadius: BorderRadius.circular(12),
                              borderSide: const BorderSide(
                                color: Color(0xFF4CAF50),
                                width: 1.5,
                              ),
                            ),
                          ),
                        );
                      },
                    ),
                  ),
                ],
              ),
              const Spacer(),
              ValueListenableBuilder<TextEditingValue>(
                valueListenable: _numberController,
                builder: (context, value, _) {
                  final canContinue = value.text.length >= 10;
                  return SizedBox(
                    height: 52,
                    child: FilledButton(
                      onPressed: canContinue ? _onVerify : null,
                      style: FilledButton.styleFrom(
                        backgroundColor: const Color(0xFF4CAF50),
                        disabledBackgroundColor: const Color(
                          0xFF4CAF50,
                        ).withValues(alpha: 0.4),
                        shape: RoundedRectangleBorder(
                          borderRadius: BorderRadius.circular(999),
                        ),
                      ),
                      child: const Text(
                        'Verify',
                        style: TextStyle(
                          fontSize: 16,
                          fontWeight: FontWeight.w700,
                          color: Colors.white,
                        ),
                      ),
                    ),
                  );
                },
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _CountryCodeBox extends StatelessWidget {
  const _CountryCodeBox({required this.flag, required this.code});

  final String flag;
  final String code;

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 52,
      padding: const EdgeInsets.symmetric(horizontal: 12),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: const Color(0xFFE0E0E0)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(flag, style: const TextStyle(fontSize: 16)),
          const SizedBox(width: 6),
          Text(
            code,
            style: const TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w600,
              color: Color(0xFF1A1A1A),
            ),
          ),
          const SizedBox(width: 4),
          const Icon(
            Icons.keyboard_arrow_down,
            size: 18,
            color: Color(0xFF888888),
          ),
        ],
      ),
    );
  }
}
