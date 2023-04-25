import _ from 'lodash';

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import Alert from 'react-bootstrap/Alert';
import Form from 'react-bootstrap/Form';
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import './estimate.css';
import T from '../i8n/i8n.jsx';

export default class EstimateHunt extends Component {
    static propTypes = {
        params: PropTypes.object,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.fetchEstimate();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let hunt_request = this.makeHuntRequest(this.props.params);
        if (!_.isEqual(this.state.prev, hunt_request)) {
            this.fetchEstimate();
            return true;
        }
        return false;
    }

    state = {
        clients_affected: 0,

        // Default to calculating all clients because this is slightly
        // faster.
        last_active: "all",
        prev: {},
    }

    getLastActive = ()=>{
        switch(this.state.last_active) {
        case "1 Month":
            return 31 * 60 * 60 * 24;

        case "1 Week":
            return 7 * 60 * 60 * 24;

        case "1 Day":
            return 60 * 60 * 24;

        default:
            return 0;
        }
    }

    makeHuntRequest = (hunt_parameters)=>{
        let result = {
            last_active: this.getLastActive(),
            condition: {},
        };
        if (hunt_parameters.include_condition === "labels") {
            result.condition.labels = {"label": hunt_parameters.include_labels};
        }
        if (hunt_parameters.include_condition === "os" &&
            hunt_parameters.include_os !== "ALL") {
            result.condition.os = {"os": hunt_parameters.include_os};
        }

        if (hunt_parameters.exclude_condition === "labels") {
            result.condition.excluded_labels = {label: hunt_parameters.excluded_labels};
        }

        return result;
    }

    fetchEstimate = () => {
        let hunt_request = this.makeHuntRequest(this.props.params);
        this.setState({prev: hunt_request});

        api.post("v1/EstimateHunt",
                 hunt_request,
                 this.source.token).then(response=>{
                     this.setState({
                         clients_affected: response.data.total_clients_scheduled
                     });
                 });
    }

    render() {
        return (
            <>
              <Alert className="estimate" variant="info">
                <Form.Group as={Row}>
                  <Form.Label column sm="8">
                    {T("Estimated affected clients")} { this.state.clients_affected || "0" }
                  </Form.Label>
                  <Col sm="3">
                    <Form.Control as="select"
                                  value={this.state.last_active}
                                  onChange={e=>this.setState({
                                      last_active: e.currentTarget.value
                                  })}
                    >
                      <option label={T("All known Clients")} value="all">
                        {T("All Known Clients")}
                      </option>
                      <option label={T("1 Day actives")} value="1 Day">
                        {T("1 Day actives")}
                      </option>
                      <option label={T("1 Week actives")} value="1 Week">
                        {T("1 Week actives")}
                      </option>
                      <option label={T("1 Month actives")} value="1 Month">
                        {T("1 Month actives")}
                      </option>
                    </Form.Control>
                  </Col>
                </Form.Group>
              </Alert>
            </>
        );
    }
}
