import { LiveDevices } from "./devices-live";
import visual from "../visual-dashboard.module.css";

export default function DevicesPage() {
  return <section className={visual.page}><header className={visual.head}><div className={visual.headMeta}><h1>Devices</h1><span className={visual.online}>Online</span></div></header><LiveDevices /></section>;
}
