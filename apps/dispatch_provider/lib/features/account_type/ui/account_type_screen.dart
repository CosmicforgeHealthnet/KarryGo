import 'package:flutter/material.dart';

class AccountTypeScreen extends StatefulWidget {
  const AccountTypeScreen({super.key, required this.onSelected});
  final ValueChanged<String> onSelected;

  @override
  State<AccountTypeScreen> createState() => _AccountTypeScreenState();
}

class _AccountTypeScreenState extends State<AccountTypeScreen> {
  String? _selected;

  static const _options = [
    _AccountOption(
      id: 'customer',
      title: 'Send, Ride or Request',
      subtitle: 'Book deliveries, rides or transport services anytimes.',
      image: 'assets/figma/account_customer.png',
    ),
    _AccountOption(
      id: 'business',
      title: 'Business Deliveries',
      subtitle: 'Dispatch your customer goods or orders.',
      image: 'assets/figma/account_delivery.png',
    ),
    _AccountOption(
      id: 'driver',
      title: 'Drive, Deliver or Haul',
      subtitle: 'Earn by completing deliveries, trips or transport jobs.',
      image: 'assets/figma/account_driver.png',
    ),
  ];

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFFF9F9F9),
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 40, 24, 32),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const Text(
                'How do you want to use KarryGo?',
                style: TextStyle(
                  fontFamily: 'Montserrat',
                  fontSize: 22,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 12),
              const Text(
                'By selecting your preferred account type, you have automatically set user role.',
                style: TextStyle(
                  fontFamily: 'Montserrat',
                  fontSize: 13,
                  color: Color(0xFF8A8A8F),
                  height: 1.4,
                ),
              ),
              const SizedBox(height: 36),
              Align(
                alignment: Alignment.center,
                child: ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 390),
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: _options
                        .map((option) => _OptionCard(
                              option: option,
                              isSelected: _selected == option.id,
                              onTap: () =>
                                  setState(() => _selected = option.id),
                            ))
                        .toList(),
                  ),
                ),
              ),
              const Spacer(),
              SizedBox(
                width: double.infinity,
                height: 54,
                child: FilledButton(
                  onPressed: _selected != null
                      ? () => widget.onSelected(_selected!)
                      : null,
                  style: FilledButton.styleFrom(
                    backgroundColor: const Color(0xFF27A747),
                    disabledBackgroundColor: const Color(0xFFBCDFCD),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(27),
                    ),
                    elevation: 0,
                  ),
                  child: Text(
                    'Continue',
                    style: TextStyle(
                      fontFamily: 'Montserrat',
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                      color: _selected != null
                          ? Colors.white
                          : Colors.white.withOpacity(0.9),
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

class _OptionCard extends StatelessWidget {
  const _OptionCard({
    required this.option,
    required this.isSelected,
    required this.onTap,
  });

  final _AccountOption option;
  final bool isSelected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 200),
        margin: const EdgeInsets.only(bottom: 20),
        width: 390,
        height: 90,
        padding: const EdgeInsets.fromLTRB(28, 10, 11, 10),
        decoration: BoxDecoration(
          color: isSelected ? const Color(0x4D27A747) : Colors.white,
          borderRadius: BorderRadius.circular(50),
          border: Border.all(color: Colors.transparent, width: 0),
          boxShadow: [
            BoxShadow(
              color: Colors.black.withOpacity(0.04),
              blurRadius: 14,
              offset: const Offset(0, 6),
            ),
          ],
        ),
        child: Row(
          children: [
            SizedBox(
              width: 52,
              height: 52,
              child: Image.asset(
                option.image,
                fit: BoxFit.contain,
                errorBuilder: (context, error, stackTrace) => Container(
                  decoration: BoxDecoration(
                    color: const Color(0xFF27A747).withOpacity(0.1),
                    shape: BoxShape.circle,
                  ),
                  child: const Icon(
                    Icons.account_box,
                    color: Color(0xFF27A747),
                  ),
                ),
              ),
            ),
            const SizedBox(width: 10),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
              Text(
  option.title,
  style: const TextStyle(
    fontFamily: 'Montserrat',
    fontSize: 18,
    fontWeight: FontWeight.w600,
    color: Color(0xFF1A1A1A),
    height: 20 / 18,
  ),
),
const SizedBox(height: 3),
Text(
  option.subtitle,
  style: const TextStyle(
    fontFamily: 'Poppins',
    fontSize: 12,
    fontWeight: FontWeight.w400,
    color: Color(0xFF888888),
    height: 20 / 12,
  ),
),
                ],
              ),
            ),
            const SizedBox(width: 10),
            Icon(
              Icons.arrow_forward,
              color: isSelected
                  ? const Color(0xFF27A747)
                  : const Color(0xFFE5E5EA),
              size: 18,
            ),
          ],
        ),
      ),
    );
  }
}

class _AccountOption {
  const _AccountOption({
    required this.id,
    required this.title,
    required this.subtitle,
    required this.image,
  });

  final String id;
  final String title;
  final String subtitle;
  final String image;
}