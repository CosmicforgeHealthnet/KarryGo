import Navbar from "@/components/Navbar";
import Hero from "@/components/Hero";
import Ecosystem from "@/components/Ecosystem";
import Services from "@/components/Services";
import OneAccount from "@/components/OneAccount";
import HowItWorks from "@/components/HowItWorks";
import Commerce from "@/components/Commerce";
import Providers from "@/components/Providers";
import Fleet from "@/components/Fleet";
import Vendors from "@/components/Vendors";
import Trust from "@/components/Trust";
import Launch from "@/components/Launch";
import Faq from "@/components/Faq";
import FinalCta from "@/components/FinalCta";
import Footer from "@/components/Footer";

export default function Home() {
  return (
    <>
      <Navbar />
      <main>
        <Hero />
        <Ecosystem />
        <Services />
        <OneAccount />
        <HowItWorks />
        <Commerce />
        <Providers />
        <Fleet />
        <Vendors />
        <Trust />
        <Launch />
        <Faq />
        <FinalCta />
      </main>
      <Footer />
    </>
  );
}