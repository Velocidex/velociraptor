import _ from 'lodash';
import "./context.css";

import UserConfig from '../core/user.jsx';
import React, { Component } from 'react';
import PropTypes from 'prop-types';
import {
  Menu,
  Item,
  useContextMenu,
} from "react-contexify";
import "react-contexify/dist/ReactContexify.css";
import T from '../i8n/i8n.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

const MENU_ID = "menu-id";

function guessValue(x) {
    if(_.isObject(x) && x.SHA256 ) {
        return x.SHA256;
    }

    if(_.isString(x)) {
        return x;
    }

    return JSON.stringify(x);
}

export default function ContextMenu({children, value}) {
  const { show } = useContextMenu({
      id: MENU_ID,
      props: {
          value: guessValue(value),
      },
  });

  return (
      <div
        className="context-menu-available"
        onContextMenu={show}>
        {children}
      </div>
  );
}

// Render the main popup menu on the root DOM page. Should only be
// called once.
export class ContextMenuPopup extends Component {
    static contextType = UserConfig;

    static propTypes = {
        value: PropTypes.any,
    }


    // https://stackoverflow.com/questions/133925/javascript-post-request-like-a-form-submit
    post(path, params, method='post') {

        // The rest of this code assumes you are not using a library.
        // It can be made less verbose if you use one.
        const form = document.createElement('form');
        form.method = method;
        form.action = path;
        form.target = "_blank";

        for (const key in params) {
            if (params.hasOwnProperty(key)) {
                const hiddenField = document.createElement('input');
                hiddenField.type = 'hidden';
                hiddenField.name = key;
                hiddenField.value = params[key];

                form.appendChild(hiddenField);
            }
        }

        document.body.appendChild(form);
        form.submit();
    }

    handleClick(link, value) {
        if (!link || !value) {
            return;
        }

        // Check if the cell has a subselection inside it.
        let selection = document.getSelection();
        if (selection && selection.type === "Range") {
            value = selection.toString();
        }

        if (!_.isString(value)) {
            value = JSON.stringify(value);
        }

        // Encode the value
        switch(link.encode) {
        case "base64":
            try {
                value = btoa(value).replaceAll('=','');
            } catch(e) {}
            break;
        default:
        }

        let params = {};
        let url = link.url;

        if (link.parameter) {
            params[link.parameter]=value;
        } else {
            url += encodeURIComponent(value);
        }
        let method = link.method || "";
        this.post(url, params, method);
    };

    copyToClipboard = (cell)=>{
        if (!cell) return;

        // Check if the cell has a subselection inside it.
        let selection = document.getSelection();
        if (selection && selection.type === "Range") {
            navigator.clipboard.writeText(selection.toString());
            return;
        }

        // If not the whole cell is highlighted then copy the entire
        // cell.
        navigator.clipboard.writeText(cell);
    }

    render() {
        let context_links = _.filter(
            this.context.traits ? this.context.traits.links : [],
            x=>x.type === "context" && !x.disabled);

        return <Menu id={MENU_ID}>
                 <Item
                   onClick={e=>{
                       if (e.props) {
                           this.copyToClipboard(e.props.value);
                       }}}>
                   <FontAwesomeIcon className="context-icon" icon="copy"/>
                   {T("Clipboard")}
                 </Item>
                 {_.map(context_links, x=>{
                     return (
                         <Item
                           key={x.text}
                           onClick={(e)=>this.handleClick(x, e.props && e.props.value)}>
                           {x.icon_url &&
                            <span>
                              <img className="context-icon"
                                   alt={x.text}
                                   src={x.icon_url}/>
                            </span>}
                           {x.text}
                         </Item>
                     );
                 })}
               </Menu>;

    }
}
