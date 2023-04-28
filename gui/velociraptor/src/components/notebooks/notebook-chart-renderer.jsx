import _ from 'lodash';
import React from 'react';
import PropTypes from 'prop-types';

import { VeloLineChart, VeloTimeChart,
         VeloScatterChart, VeloBarChart } from '../artifacts/line-charts.jsx';
import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';
import { PrepareData } from '../core/table.jsx';


export class NotebookLineChart extends React.Component {
    static propTypes = {
        params: PropTypes.object,
    };

    state = {
        rows: [],
        columns: [],
        loading: true,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.fetchRows();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    componentDidUpdate(prevProps) {
        let previous = prevProps.params && prevProps.params.Version;
        let current = this.props.params && this.props.params.Version;
        if (previous !== current) {
           this.fetchRows();
        }
    }

    fetchRows = () => {
        if (_.isEmpty(this.props.params)) {
            this.setState({loading: false});
            return;
        }


        let params = Object.assign({}, this.props.params);
        let url = "v1/GetTable";

        this.source.cancel();
        this.source = CancelToken.source();

        api.get(url, params, this.source.token).then((response) => {
            if (response.cancel) {
                return;
            }

            let pageData = PrepareData(response.data);
            let columns = pageData.columns;
            this.setState({loading: false,
                           rows: pageData.rows,
                           columns: columns });
        }).catch((e) => {
            this.setState({loading: false, rows: [], columns: []});
        });
    }

    render() {
        if (_.isEmpty(this.state.rows)) {
            return <></>;
        }
        return <VeloLineChart
                 className="col-12"
                 params={this.props.params}
                 columns={this.state.columns}
                 data={this.state.rows}
               />;
    }
};

export class NotebookTimeChart extends NotebookLineChart {
   render() {
        if (_.isEmpty(this.state.rows)) {
            return <></>;
        }
        return <VeloTimeChart
                 className="col-12"
                 params={this.props.params}
                 columns={this.state.columns}
                 data={this.state.rows}
               />;
    }
}

export class NotebookBarChart extends NotebookLineChart {
   render() {
        if (_.isEmpty(this.state.rows)) {
            return <></>;
        }
        return <VeloBarChart
                 className="col-12"
                 params={this.props.params}
                 columns={this.state.columns}
                 data={this.state.rows}
               />;
    }
}

export class NotebookScatterChart extends NotebookLineChart {
   render() {
        if (_.isEmpty(this.state.rows)) {
            return <></>;
        }
        return <VeloScatterChart
                 className="col-12"
                 params={this.props.params}
                 columns={this.state.columns}
                 data={this.state.rows}
               />;
    }
}
