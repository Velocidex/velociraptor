import React, { Component } from 'react';
import PropTypes from 'prop-types';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { withRouter, Link }  from "react-router-dom";
import ShellViewer from "../clients/shell-viewer.jsx";
import MetadataEditor from "../clients/metadata.jsx";

import ToggleButtonGroup from 'react-bootstrap/ToggleButtonGroup';
import ToggleButton from 'react-bootstrap/ToggleButton';
import Button from 'react-bootstrap/Button';
import T from '../i8n/i8n.jsx';
import Card from 'react-bootstrap/Card';
import Row from 'react-bootstrap/Row';
import ToolTip from '../widgets/tooltip.jsx';

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
                    <Card.Header>Server configuration</Card.Header>
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
                <div className="client-info">
                  <div className="btn-group float-left toolbar" data-toggle="buttons">
                    <Link to={"/collected/server"}
                          role="button" className="btn btn-default">
                      <i><FontAwesomeIcon icon="history"/></i>
                      <span className="button-label">Collected</span>
                    </Link>
                  </div>

                  <ToggleButtonGroup type="radio"
                                     name="mode"
                                     defaultValue={this.state.mode}
                                     onChange={(mode) => this.setMode(mode)}
                                     className="mb-2">
                    <ToggleButton variant="default"
                                  value='brief'>
                      <FontAwesomeIcon icon="laptop"/>
                      <span className="button-label">Overview</span>
                    </ToggleButton>
                    <ToggleButton variant="default"
                                  value='shell'>
                      <FontAwesomeIcon icon="terminal"/>
                      <span className="button-label">Server Shell</span>
                    </ToggleButton>
                  </ToggleButtonGroup>
                </div>
                <div className="clearfix"></div>
                { this.renderContent() }
              </div>
            </>
        );
    };
}

export default withRouter(ServerInfo);
