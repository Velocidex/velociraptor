import React, { Component } from 'react';

import _ from 'lodash';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { withRouter }  from "react-router-dom";
import ShellViewer from "../clients/shell-viewer.js";
import VeloForm from '../forms/form.js';

import ToggleButtonGroup from 'react-bootstrap/ToggleButtonGroup';
import ToggleButton from 'react-bootstrap/ToggleButton';
import CardDeck from 'react-bootstrap/CardDeck';
import Card from 'react-bootstrap/Card';

import { Link } from  "react-router-dom";
import api from '../core/api-service.js';
import axios from 'axios';
import { parseCSV } from '../utils/csv.js';
import "../clients/host-info.css";

const POLL_TIME = 5000;


class ServerInfo extends Component {
    static propTypes = {}

    state = {
        // The mode of the host info tab set.
        mode: this.props.match.params.action || 'brief',

        metadata: "Key,Value\n,\n",

        loading: false,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.interval = setInterval(this.fetchMetadata, POLL_TIME);
        this.fetchMetadata();
    }

    componentWillUnmount() {
        this.source.cancel();
        clearInterval(this.interval);
        if (this.interrogate_interval) {
            clearInterval(this.interrogate_interval);
        }
    }

    fetchMetadata = () => {
        this.setState({loading: true});

        this.source.cancel();
        this.source = axios.CancelToken.source();

        api.get("v1/GetClientMetadata/server",
                {}, this.source.token).then(response=>{
                    if (response.cancel) return;

                    let metadata = "Key,Value\n";
                    var rows = 0;
                    var items = response.data["items"] || [];
                    for (var i=0; i<items.length; i++) {
                        var key = items[i]["key"] || "";
                        var value = items[i]["value"] || "";
                        if (!_.isUndefined(key)) {
                            metadata += key + "," + value + "\n";
                            rows += 1;
                        }
                    };
                    if (rows === 0) {
                        metadata = "Key,Value\n,\n";
                    };
                    this.setState({metadata: metadata, loading: false});
                });
    }

    setMetadata = (value) => {
        var data = parseCSV(value);
        let items = _.map(data.data, (x) => {
            return {key: x.Key, value: x.Value};
        });

        var params = {
            client_id: "server",
            items: items,
        };
        api.post("v1/SetClientMetadata", params).then(() => {
            this.fetchMetadata();
        });
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
                <CardDeck className="dashboard">
                  <Card>
                    <Card.Header>Server configuration</Card.Header>
                    <Card.Body>
                      <VeloForm
                        param={{type: "csv", name: "Server Metadata"}}
                        value={this.state.metadata}
                        setValue={this.setMetadata}
                      />
                    </Card.Body>
                  </Card>
                </CardDeck>
            );
        };

        if (this.state.mode === 'shell') {
            return (
                <div className="client-details shell">
                  <ShellViewer client={{client_id: "server"}}
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
