import _ from 'lodash';

import {
  Menu,
  Item,
  Separator,
  Submenu,
  useContextMenu,
} from "react-contexify";
import "react-contexify/dist/ReactContexify.css";

const MENU_ID = "menu-id";

export default function ContextMenu({children, value}) {
  const { show } = useContextMenu({
      id: MENU_ID,
      props: {
          value: value,
      },
  });

  return (
    <div>
      <div onContextMenu={show}>
        {children}
      </div>
    </div>
  );
}

// Render the main popup menu on the root DOM page. Should only be
// called once.
export function ContextMenuPopup() {
    function handleItemClick({ event, props, triggerEvent, data }){
        console.log(event, props, triggerEvent, data );
    }

    // Get the context menu options from the server - this will be
    // injected into window.applications from GUI.Applications from
    // the config file.
    let applications = {};
    try {
        applications = JSON.parse(window.applications);
    } catch(e) {};

    // Set a default URL - the user may override with custom
    // CyberChef server.
    let cyberchef = applications.cyberchef_url ||
        "https://gchq.github.io/CyberChef/#input=";

    function handleCyberChef({e, props}) {
        let value = props.value || "";
        if (!_.isString(value)) {
            value = JSON.stringify(value);
        }
        if (value) {
            try {
                let encoded = btoa(value).replaceAll('=','');
                window.open(cyberchef + encoded);
            } catch(e) {}
        }
    };

    return (
      <Menu id={MENU_ID}>
        <Item onClick={handleCyberChef}>
          Send to CyberChef
        </Item>
      </Menu>
    );
}
