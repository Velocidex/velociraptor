import './keyboard-help.css';
import _ from 'lodash';
import React from 'react';
import Button from 'react-bootstrap/Button';
import { GlobalHotKeys } from "react-hotkeys";
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

const KeyMap = {
    SHOW_HELP: "ctrl+/",
};

const helpTextCol1 = [
    ["Global hotkeys", [
        ["ctrl+d", "Goto dashboard"],
        ["alt+n", "Goto notebooks"],
        ["ctrl+c", "Collected artifacts"],
        ["ctrl+/", "Show/Hide keyboard hotkeys help"],
    ]],

    ["New Artifact Collection Wizard", [
        ["alt+a", "Artifact Selection Step"],
        ["alt+p", "Parameters configuration Step"],
        ["alt+r", "Collection resource specification"],
        ["ctrl+l", "Launch artifact"],
        ["ctrl+right", "Go to next step"],
        ["ctrl+left", "Go to previous step"],
    ]],
];

const helpTextCol2 = [
    ["Collected Artifacts", [
        ["n", "Select next collection"],
        ["p", "Select previous collection"],
        ["r", "View selected collection results"],
        ["o", "View selected collection overview"],
        ["l", "View selected collection logs"],
        ["u", "View selected collection uploaded files"],
    ]],
    ["Editor shortcuts", [
        ["ctrl+,", "Popup the editor configuration dialog"],
    ]],
];

export default class KeyboardHelp extends React.PureComponent {
    state = {
        showHelp: false,
    }

    renderKey = (key) => {
        let parts = key.split("+");
        switch (parts[0]) {
        case "alt":
        case "ctrl":
            return <>
                     <span className="highlight ctrl">&lt;{parts[0]}&gt;</span>
                     <span className=""> + </span>
                     <span className="highlight">{parts[1]}</span>
                   </>;
        default:
            return <span className="highlight">{key}</span>;
        };
    }

    makeColumn = (specs) => {
        return <table>
                 <tbody>
                   { _.map(specs, (spec, v)=>{
                       let title = spec[0];
                       let desc = spec[1];

                       return (<React.Fragment key={v}>
                                <tr><td></td>
                                  <td className="heading">
                                    {title}
                                  </td></tr>
                                { _.map(desc, (x, i)=>{
                                    return <tr key={i}>
                                              <td className="key">
                                                {this.renderKey(x[0])}  :
                                              </td>
                                              <td className="desc">{x[1]}</td>
                                            </tr>;
                                })}
                               </React.Fragment>);
                   })}
                 </tbody>
               </table>;
    }

    render() {
        return (
            <>
              <GlobalHotKeys keyMap={KeyMap}
                             handlers={{
                                 SHOW_HELP: (e)=> {
                                     this.setState({showHelp: !this.state.showHelp});
                                     e.preventDefault();
                                 },
                             }} />
              { this.state.showHelp &&
                <>
                <GlobalHotKeys keyMap={{ESCAPE: "esc"}}
                         handlers={{
                             ESCAPE: ()=>this.setState({showHelp: false}),
                         }} />
                <div className="keyboard-help"
                     onClick={()=>this.setState({showHelp: false})}>
                  <div className="keyboard-help-content">
                    <table className="page-heading">
                      <tbody>
                        <tr><td>Keyboard shortcuts</td>
                          <td><Button size="lg"
                                className="float-right"
                                variant="link">
                                <FontAwesomeIcon icon="window-close"/>
                              </Button></td>
                        </tr>
                      </tbody>
                    </table>
                    <div className="help-content">
                      <table cellPadding="0">
                        <tbody>
                          <tr>
                            <td className="column">
                              { this.makeColumn(helpTextCol1)}
                            </td>
                            <td className="column">
                              { this.makeColumn(helpTextCol2)}
                            </td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  </div>
                </div>
                </>
            }
            </>
        );
    }
};
