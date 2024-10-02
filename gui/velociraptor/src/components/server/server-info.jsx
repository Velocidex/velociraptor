import React, { Component } from 'react';
import PropTypes from 'prop-types';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { withRouter, Link }  from "react-router-dom";
import ShellViewer from "../clients/shell-viewer.jsx";
import MetadataEditor from "../clients/metadata.jsx";
import classNames from "classnames";

import Button from 'react-bootstrap/Button';
import T from '../i8n/i8n.jsx';
import Card from 'react-bootstrap/Card';
import Row from 'react-bootstrap/Row';
import ToolTip from '../widgets/tooltip.jsx';
import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';

import {CancelToken} from 'axios';
import "../clients/host-info.css";

class ServerInfo extends Component {
    static propTypes = {
        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    }

    state = {
        // The mode of the host info tab set.
        mode: this.props.match.params.action || 'brief',

        loading: false,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel();
        if (this.interrogate_interval) {
            clearInterval(this.interrogate_interval);
        }
    }

    setMode = (mode) => {
        if (mode !== this.state.mode) {
            let new_state  = Object.assign({}, this.state);
            new_state.mode = mode;
            this.setState(new_state);

            this.props.history.push('/host/server/' + mode);
        }
    }

    renderContent = () => {
        if (this.state.mode === 'brief') {
            return (
                <Row className="dashboard">
                  <Card>
                    <Card.Header>{T("Server configuration")}</Card.Header>
                    <Card.Body>
                      <MetadataEditor
                        valueRenderer={(cell, row)=>{
                            return <ToolTip tooltip={T("Click to view or edit")}>
                                     <Button variant="default-outline" size="sm">
                                       <FontAwesomeIcon icon="wrench"/>
                                     </Button>
                                   </ToolTip>;
                        }}
                        client_id="server" />
                    </Card.Body>
                  </Card>
                </Row>
            );
        };

        if (this.state.mode === 'shell') {
            return (
                <div className="client-details shell">
                  <ShellViewer
                    client={{client_id: "server"}}
                    default_shell="Bash"
                  />
                </div>
            );
        }

        return <div>Unknown mode</div>;
    }

    render() {
        return (
            <>
              <div className="full-width-height">
                <Navbar className="toolbar">
                  <ButtonGroup className="" data-toggle="buttons">
                    <Link to={"/collected/server"}
                          role="button" className="btn btn-default">
                      <i><FontAwesomeIcon icon="history"/></i>
                      <span className="button-label">{T("Collected")}</span>
                    </Link>
                  </ButtonGroup>
                  <ButtonGroup className="float-right">
                    <Button variant="default"
                            className={classNames({
                                active: this.state.mode === "brief"})}
                            onClick={(mode) => this.setMode("brief")}>
                      <FontAwesomeIcon icon="laptop"/>
                      <span className="button-label">{T("Overview")}</span>
                    </Button>
                    <Button variant="default"
                            className={classNames({
                                active: this.state.mode === "shell"})}
                            onClick={(mode) => this.setMode("shell")}>
                      <FontAwesomeIcon icon="terminal"/>
                      <span className="button-label">{T("Server Shell")}</span>
                    </Button>
                  </ButtonGroup>
              </Navbar>
                <div className="clearfix"></div>
                { this.renderContent() }
              </div>
            </>
        );
    };
}

export default withRouter(ServerInfo);
