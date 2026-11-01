import Navbar from "@/components/Navbar";
import Hero from "@/components/Hero";
import Ecosystem from "@/components/Ecosystem";
import Services from "@/components/Services";
import OneAccount from "@/components/OneAccount";
import HowItWorks from "@/components/HowItWorks";
import Commerce from "@/components/Commerce";
import Providers from "@/components/Providers";
import DriverApp from "@/components/DriverApp";
import SectionJoin from "@/components/SectionJoin";
import Fleet from "@/components/Fleet";
import Vendors from "@/components/Vendors";
import Trust from "@/components/Trust";
import Launch from "@/components/Launch";
import Faq from "@/components/Faq";
import FinalCta from "@/components/FinalCta";
import Footer from "@/components/Footer";

const MIDNIGHT = "#0B1F44";
const WHITE = "#FFFFFF";

export default function Home() {
  return (
    <>
      <Navbar />
      <main>
        <Hero />
        <SectionJoin from={MIDNIGHT} to={WHITE} />
        <Ecosystem />
        <Services />
        <SectionJoin from={WHITE} to={MIDNIGHT} />
        <OneAccount />
        <SectionJoin from={MIDNIGHT} to={WHITE} />
        <HowItWorks />
        <Commerce />
        <Providers />
        <SectionJoin from={WHITE} to={MIDNIGHT} />
        <DriverApp />
        <SectionJoin from={MIDNIGHT} to={WHITE} />
        <Fleet />
        <Vendors />
        <Trust />
        <SectionJoin from="#F3F8FB" to={MIDNIGHT} />
        <Launch />
        <SectionJoin from={MIDNIGHT} to={WHITE} />
        <Faq />
        <SectionJoin from={WHITE} to={MIDNIGHT} />
        <FinalCta />
      </main>
      <Footer />
    </>
  );
}