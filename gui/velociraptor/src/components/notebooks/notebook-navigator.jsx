import _ from 'lodash';

import "./notebook-navigator.css";
import Button from 'react-bootstrap/Button';
import PropTypes from 'prop-types';
import React, { Component } from 'react';
import T from '../i8n/i8n.jsx';
import VeloTable, { getFormatter } from '../core/table.jsx';
import VeloTimestamp from "../utils/time.jsx";
import classNames from "classnames";
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { withRouter }  from "react-router-dom";

class NotebookNavigator extends Component {
    static propTypes = {
        notebook: PropTypes.object,
        scrollToCell: PropTypes.func,
        match: PropTypes.object,
        history: PropTypes.object,
    };

    state = {
        collapsed: true,
    }

    linkToCell = (cell, row)=>{
        return <Button
                 onClick={x=>{
                     this.props.scrollToCell(cell);
                     this.setState({collapsed: true});
                 }}
               >
                 {cell}
               </Button>;
    }

    setFullScreen = () => {
        if (this.props.notebook &&
            this.props.notebook.notebook_id) {

            console.log(this.props.match);
            if (this.props.match.path.startsWith("/fullscreen")) {
                this.props.history.push(
                    "/notebooks/" +
                        this.props.notebook.notebook_id);
            } else {
                this.props.history.push(
                    "/fullscreen/notebooks/" +
                        this.props.notebook.notebook_id);
            }
        }
    }

    render() {
        let md = (this.props.notebook &&
                  this.props.notebook.cell_metadata) || [];

        return (
            <div className="float-left navigator">
              <div
                className={classNames({
                    collapsed: this.state.collapsed,
                    uncollapsed: !this.state.collapsed,
                    notebook_nav: true,
                })}
                onClick={this.collapse}
              >
                <div>
                <nav className="navigator" aria-labelledby="mainmenu">
                  <ul className="nav nav-pills navigator">
                    <li className="nav-link">
                      <Button
                        onClick={this.setFullScreen}
                        variant='outline-default'>
                        <i className="navicon">
                          <FontAwesomeIcon icon="expand" />
                        </i>
                      </Button>
                    </li>
                    <li className="nav-link">
                      <Button
                        onClick={x=>this.setState({
                            collapsed: !this.state.collapsed})}
                        variant='outline-default'>
                          <i className="navicon">
                            <FontAwesomeIcon icon="list" />
                          </i>
                      </Button>
                      <div className="link-description">
                        {T("Overview")}
                      </div>
                    </li>
                    <span className="notebook-outline">
                      <VeloTable
                        rows={md}
                        columns={["cell_id", "timestamp", "type", "summary"]}
                        no_toolbar={md.length < 10}
                        header_renderers={{"cell_id": T("Cell"),
                                           "timestamp": T("Timestamp"),
                                           "type": T("Type")}}
                        column_renderers={{
                            "cell_id": this.linkToCell,
                            "timestamp": getFormatter("timestamp")}}
                      />
                    </span>
                  </ul>
                </nav>
            </div>
            </div>
            </div>
        );
    }
}


export default withRouter(NotebookNavigator);
