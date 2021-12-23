import _ from 'lodash';

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import Alert from 'react-bootstrap/Alert';

import api from '../core/api-service.js';
import axios from 'axios';

import './estimate.css';

export default class EstimateHunt extends Component {
    static propTypes = {
        params: PropTypes.object,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
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
        prev: {},
    }

    makeHuntRequest = (hunt_parameters)=>{
        let result = {
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
                Estimated affected clients { this.state.clients_affected || "0" }
              </Alert>
            </>
        );
    }
}
